package planar

import (
	"context"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/bosley/txpx/pkg/pool"
)

type mockKV struct {
	mu   sync.RWMutex
	data map[string][]byte
}

func newMockKV() *mockKV {
	return &mockKV{
		data: make(map[string][]byte),
	}
}

func (m *mockKV) Get(key []byte) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	value, exists := m.data[string(key)]
	if !exists {
		return nil, nil
	}

	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

func (m *mockKV) Set(key []byte, value []byte) (KVMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keyStr := string(key)
	oldValue, existed := m.data[keyStr]

	newValue := make([]byte, len(value))
	copy(newValue, value)
	m.data[keyStr] = newValue

	result := KVMutationResult{}
	if !existed {
		result.UniqueKeyBytesDelta = int64(len(key))
		result.UniqueValueBytesDelta = int64(len(value))
	} else {
		result.UniqueValueBytesDelta = int64(len(value) - len(oldValue))
	}

	return result, nil
}

func (m *mockKV) Delete(key []byte) (KVMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keyStr := string(key)
	oldValue, existed := m.data[keyStr]

	result := KVMutationResult{}
	if existed {
		delete(m.data, keyStr)
		result.UniqueKeyBytesDelta = -int64(len(key))
		result.UniqueValueBytesDelta = -int64(len(oldValue))
	}

	return result, nil
}

func (m *mockKV) Exists(key []byte) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.data[string(key)]
	return exists, nil
}

func (m *mockKV) IterateKeys(prefix []byte, offset uint32, limit uint32, recv KVIPIterRecv) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefixStr := string(prefix)
	var keys []string

	for key := range m.data {
		if len(prefix) == 0 || len(key) >= len(prefixStr) && key[:len(prefixStr)] == prefixStr {
			keys = append(keys, key)
		}
	}

	start := int(offset)
	if start >= len(keys) {
		return nil
	}

	end := start + int(limit)
	if end > len(keys) || limit == 0 {
		end = len(keys)
	}

	for i := start; i < end; i++ {
		cont, err := recv([]byte(keys[i]))
		if err != nil {
			return err
		}
		if !cont {
			break
		}
	}

	return nil
}

func (m *mockKV) SetBatch(items []KVItem) (KVBatchMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := KVBatchMutationResult{}

	for _, item := range items {
		keyStr := string(item.Key)
		oldValue, existed := m.data[keyStr]

		newValue := make([]byte, len(item.Value))
		copy(newValue, item.Value)
		m.data[keyStr] = newValue

		result.KeysUpdated++
		if !existed {
			result.UniqueKeyBytesDelta += int64(len(item.Key))
			result.UniqueValueBytesDelta += int64(len(item.Value))
		} else {
			result.UniqueValueBytesDelta += int64(len(item.Value) - len(oldValue))
		}
	}

	return result, nil
}

func (m *mockKV) DeleteBatch(items []KVItem) (KVBatchMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	result := KVBatchMutationResult{}

	for _, item := range items {
		keyStr := string(item.Key)
		oldValue, existed := m.data[keyStr]

		if existed {
			delete(m.data, keyStr)
			result.KeysUpdated++
			result.UniqueKeyBytesDelta -= int64(len(item.Key))
			result.UniqueValueBytesDelta -= int64(len(oldValue))
		}
	}

	return result, nil
}

func (m *mockKV) CompareAndSwap(key []byte, oldValue []byte, newValue []byte) (KVMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keyStr := string(key)
	currentValue, exists := m.data[keyStr]

	result := KVMutationResult{}

	if !exists && oldValue == nil {
		newVal := make([]byte, len(newValue))
		copy(newVal, newValue)
		m.data[keyStr] = newVal
		result.UniqueKeyBytesDelta = int64(len(key))
		result.UniqueValueBytesDelta = int64(len(newValue))
		return result, nil
	} else if exists && len(currentValue) == len(oldValue) && string(currentValue) == string(oldValue) {
		newVal := make([]byte, len(newValue))
		copy(newVal, newValue)
		m.data[keyStr] = newVal
		result.UniqueValueBytesDelta = int64(len(newValue) - len(currentValue))
		return result, nil
	}

	return result, ErrKVAtomicFailureSwapValueMismatch
}

func (m *mockKV) CompareAndDelete(key []byte, oldValue []byte) (KVMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keyStr := string(key)
	currentValue, exists := m.data[keyStr]

	result := KVMutationResult{}

	if exists && string(currentValue) == string(oldValue) {
		delete(m.data, keyStr)
		result.UniqueKeyBytesDelta = -int64(len(key))
		result.UniqueValueBytesDelta = -int64(len(currentValue))
		return result, nil
	}

	return result, ErrKVAtomicFailureSwapValueMismatch
}

func (m *mockKV) SetIfNotExists(key []byte, value []byte) (KVMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	keyStr := string(key)
	_, exists := m.data[keyStr]

	result := KVMutationResult{}

	if !exists {
		newValue := make([]byte, len(value))
		copy(newValue, value)
		m.data[keyStr] = newValue
		result.UniqueKeyBytesDelta = int64(len(key))
		result.UniqueValueBytesDelta = int64(len(value))
		return result, nil
	}

	return result, ErrKVAtomicFailureKeyAlreadyExists
}

func (m *mockKV) IterateItems(options KVIterOptions, recv KVIterRecv) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	prefixStr := string(options.Prefix)

	for key, value := range m.data {
		if len(options.Prefix) == 0 || len(key) >= len(prefixStr) && key[:len(prefixStr)] == prefixStr {
			item := KVItem{
				Key:   []byte(key),
				Value: make([]byte, len(value)),
			}
			copy(item.Value, value)

			if options.RemovePrefixOnCallback && len(options.Prefix) > 0 {
				item.Key = item.Key[len(options.Prefix):]
			}

			cont, err := recv(options.Ctx, item)
			if err != nil {
				return err
			}
			if !cont {
				break
			}
		}
	}

	return nil
}

type mockWatcher struct {
	mu            sync.Mutex
	onSetCalls    []OnSetCall
	onDeleteCalls []OnDeleteCall
}

type OnSetCall struct {
	Key []byte
}

type OnDeleteCall struct {
	Key []byte
}

func newMockWatcher() *mockWatcher {
	return &mockWatcher{}
}

func (w *mockWatcher) OnSet(key []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	call := OnSetCall{
		Key: make([]byte, len(key)),
	}
	copy(call.Key, key)
	w.onSetCalls = append(w.onSetCalls, call)
}

func (w *mockWatcher) OnDelete(key []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()

	call := OnDeleteCall{
		Key: make([]byte, len(key)),
	}
	copy(call.Key, key)
	w.onDeleteCalls = append(w.onDeleteCalls, call)
}

func (w *mockWatcher) getOnSetCalls() []OnSetCall {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]OnSetCall, len(w.onSetCalls))
	copy(result, w.onSetCalls)
	return result
}

func (w *mockWatcher) getOnDeleteCalls() []OnDeleteCall {
	w.mu.Lock()
	defer w.mu.Unlock()
	result := make([]OnDeleteCall, len(w.onDeleteCalls))
	copy(result, w.onDeleteCalls)
	return result
}

func (w *mockWatcher) reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.onSetCalls = nil
	w.onDeleteCalls = nil
}

func setupVKVTest(t *testing.T) (*vkv, *mockKV, *pool.Pool, func()) {
	mockKV := newMockKV()

	logger := slog.Default()
	ctx := context.Background()

	watchPool := pool.NewBuilder().
		WithWorkers(2).
		WithLogger(logger).
		Build(ctx)

	vkv := NewVirtualKV(watchPool, mockKV, []byte("test/"))

	cleanup := func() {
		watchPool.Stop()
	}

	return vkv, mockKV, watchPool, cleanup
}

func waitForWatcherNotifications(t *testing.T, duration time.Duration) {
	time.Sleep(duration)
}

func TestVKV_BasicOperations(t *testing.T) {
	vkv, mockKV, _, cleanup := setupVKVTest(t)
	defer cleanup()

	t.Run("scope prefix handling", func(t *testing.T) {
		key := []byte("mykey")
		value := []byte("myvalue")

		_, err := vkv.Set(key, value)
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}

		rawValue, err := mockKV.Get([]byte("test/mykey"))
		if err != nil {
			t.Fatalf("Direct mockKV Get failed: %v", err)
		}
		if string(rawValue) != "myvalue" {
			t.Errorf("Expected scoped key to be stored in mockKV, got %q", rawValue)
		}

		retrievedValue, err := vkv.Get(key)
		if err != nil {
			t.Fatalf("VKV Get failed: %v", err)
		}
		if string(retrievedValue) != "myvalue" {
			t.Errorf("Expected %q, got %q", "myvalue", retrievedValue)
		}
	})
}

func TestVKV_WatcherNotifications_Set(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	key := []byte("testkey")
	value := []byte("testvalue")

	_, err := vkv.Set(key, value)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnSetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 OnSet call, got %d", len(calls))
	}

	call := calls[0]
	if string(call.Key) != "testkey" {
		t.Errorf("Expected key %q, got %q", "testkey", call.Key)
	}
}

func TestVKV_WatcherNotifications_Delete(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	key := []byte("testkey")
	value := []byte("testvalue")

	_, err := vkv.Set(key, value)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)
	watcher.reset()

	_, err = vkv.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnDeleteCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 OnDelete call, got %d", len(calls))
	}

	call := calls[0]
	if string(call.Key) != "testkey" {
		t.Errorf("Expected key %q, got %q", "testkey", call.Key)
	}
}

func TestVKV_WatcherNotifications_DeleteNonExistent(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	key := []byte("nonexistent")

	_, err := vkv.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnDeleteCalls()
	if len(calls) != 0 {
		t.Fatalf("Expected 0 OnDelete calls for non-existent key, got %d", len(calls))
	}
}

func TestVKV_WatcherNotifications_SetBatch(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	items := []KVItem{
		{Key: []byte("key1"), Value: []byte("value1")},
		{Key: []byte("key2"), Value: []byte("value2")},
		{Key: []byte("key3"), Value: []byte("value3")},
	}

	_, err := vkv.SetBatch(items)
	if err != nil {
		t.Fatalf("SetBatch failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnSetCalls()
	if len(calls) != 3 {
		t.Fatalf("Expected 3 OnSet calls for batch operation, got %d", len(calls))
	}

	expectedKeys := map[string]bool{"key1": true, "key2": true, "key3": true}
	actualKeys := make(map[string]bool)

	for _, call := range calls {
		actualKeys[string(call.Key)] = true
	}

	for expectedKey := range expectedKeys {
		if !actualKeys[expectedKey] {
			t.Errorf("Missing expected set call for key %q", expectedKey)
		}
	}

	for actualKey := range actualKeys {
		if !expectedKeys[actualKey] {
			t.Errorf("Unexpected set call for key %q", actualKey)
		}
	}
}

func TestVKV_WatcherNotifications_DeleteBatch(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	items := []KVItem{
		{Key: []byte("key1"), Value: []byte("value1")},
		{Key: []byte("key2"), Value: []byte("value2")},
	}

	_, err := vkv.SetBatch(items)
	if err != nil {
		t.Fatalf("SetBatch failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)
	watcher.reset()

	_, err = vkv.DeleteBatch(items)
	if err != nil {
		t.Fatalf("DeleteBatch failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnDeleteCalls()
	if len(calls) != 2 {
		t.Fatalf("Expected 2 OnDelete calls for batch operation, got %d", len(calls))
	}

	expectedKeys := map[string]bool{"key1": true, "key2": true}
	actualKeys := make(map[string]bool)

	for _, call := range calls {
		actualKeys[string(call.Key)] = true
	}

	for expectedKey := range expectedKeys {
		if !actualKeys[expectedKey] {
			t.Errorf("Missing expected delete call for key %q", expectedKey)
		}
	}

	for actualKey := range actualKeys {
		if !expectedKeys[actualKey] {
			t.Errorf("Unexpected delete call for key %q", actualKey)
		}
	}
}

func TestVKV_WatcherNotifications_CompareAndSwap(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	key := []byte("testkey")
	oldValue := []byte("oldvalue")
	newValue := []byte("newvalue")

	_, err := vkv.Set(key, oldValue)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)
	watcher.reset()

	_, err = vkv.CompareAndSwap(key, oldValue, newValue)
	if err != nil {
		t.Fatalf("CompareAndSwap failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnSetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 OnSet call after successful CAS, got %d", len(calls))
	}

	call := calls[0]
	if string(call.Key) != "testkey" {
		t.Errorf("Expected key %q, got %q", "testkey", call.Key)
	}
}

func TestVKV_WatcherNotifications_CompareAndSwapFailed(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	key := []byte("testkey")
	oldValue := []byte("oldvalue")
	wrongValue := []byte("wrongvalue")
	newValue := []byte("newvalue")

	_, err := vkv.Set(key, oldValue)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)
	watcher.reset()

	_, err = vkv.CompareAndSwap(key, wrongValue, newValue)
	if err == nil {
		t.Fatalf("CompareAndSwap should have failed with wrong value")
	}
	if err != ErrKVAtomicFailureSwapValueMismatch {
		t.Fatalf("Expected ErrKVAtomicFailureSwapValueMismatch, got: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnSetCalls()
	if len(calls) != 0 {
		t.Fatalf("Expected 0 OnSet calls after failed CAS, got %d", len(calls))
	}
}

func TestVKV_WatcherNotifications_CompareAndDelete(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	key := []byte("testkey")
	value := []byte("testvalue")

	_, err := vkv.Set(key, value)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)
	watcher.reset()

	_, err = vkv.CompareAndDelete(key, value)
	if err != nil {
		t.Fatalf("CompareAndDelete failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnDeleteCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 OnDelete call after successful CAD, got %d", len(calls))
	}

	call := calls[0]
	if string(call.Key) != "testkey" {
		t.Errorf("Expected key %q, got %q", "testkey", call.Key)
	}
}

func TestVKV_WatcherNotifications_SetIfNotExists(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	key := []byte("testkey")
	value := []byte("testvalue")

	_, err := vkv.SetIfNotExists(key, value)
	if err != nil {
		t.Fatalf("SetIfNotExists failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnSetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 OnSet call after successful SetIfNotExists, got %d", len(calls))
	}

	watcher.reset()

	_, err = vkv.SetIfNotExists(key, []byte("newvalue"))
	if err == nil {
		t.Fatalf("Second SetIfNotExists should have failed - key already exists")
	}
	if err != ErrKVAtomicFailureKeyAlreadyExists {
		t.Fatalf("Expected ErrKVAtomicFailureKeyAlreadyExists, got: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls = watcher.getOnSetCalls()
	if len(calls) != 0 {
		t.Fatalf("Expected 0 OnSet calls after failed SetIfNotExists, got %d", len(calls))
	}
}

func TestVKV_MultipleWatchers(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher1 := newMockWatcher()
	watcher2 := newMockWatcher()
	watcher3 := newMockWatcher()

	vkv.AddWatcher(1, watcher1)
	vkv.AddWatcher(2, watcher2)
	vkv.AddWatcher(3, watcher3)

	key := []byte("testkey")
	value := []byte("testvalue")

	_, err := vkv.Set(key, value)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	for i, watcher := range []*mockWatcher{watcher1, watcher2, watcher3} {
		calls := watcher.getOnSetCalls()
		if len(calls) != 1 {
			t.Errorf("Watcher %d: expected 1 OnSet call, got %d", i+1, len(calls))
		}
	}

	vkv.RemoveWatcher(2)

	for _, watcher := range []*mockWatcher{watcher1, watcher2, watcher3} {
		watcher.reset()
	}

	_, err = vkv.Set([]byte("key2"), []byte("value2"))
	if err != nil {
		t.Fatalf("Second Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	if len(watcher1.getOnSetCalls()) != 1 {
		t.Errorf("Watcher 1: expected 1 OnSet call, got %d", len(watcher1.getOnSetCalls()))
	}
	if len(watcher2.getOnSetCalls()) != 0 {
		t.Errorf("Watcher 2: expected 0 OnSet calls after removal, got %d", len(watcher2.getOnSetCalls()))
	}
	if len(watcher3.getOnSetCalls()) != 1 {
		t.Errorf("Watcher 3: expected 1 OnSet call, got %d", len(watcher3.getOnSetCalls()))
	}
}

func TestVKV_VirtualScope_Isolation(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	scope1 := vkv.NewVirtualScope([]byte("scope1/"))
	scope2 := vkv.NewVirtualScope([]byte("scope2/"))

	watcher1 := newMockWatcher()
	watcher2 := newMockWatcher()

	scope1.AddWatcher(1, watcher1)
	scope2.AddWatcher(1, watcher2)

	key := []byte("testkey")
	value1 := []byte("value1")
	value2 := []byte("value2")

	_, err := scope1.Set(key, value1)
	if err != nil {
		t.Fatalf("Scope1 Set failed: %v", err)
	}

	_, err = scope2.Set(key, value2)
	if err != nil {
		t.Fatalf("Scope2 Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls1 := watcher1.getOnSetCalls()
	calls2 := watcher2.getOnSetCalls()

	if len(calls1) != 1 {
		t.Fatalf("Scope1 watcher: expected 1 OnSet call, got %d", len(calls1))
	}
	if len(calls2) != 1 {
		t.Fatalf("Scope2 watcher: expected 1 OnSet call, got %d", len(calls2))
	}

	// Watchers only receive keys, not values, so we just verify the calls were made
	// The actual values can be verified through Get operations below

	retrievedValue1, err := scope1.Get(key)
	if err != nil {
		t.Fatalf("Scope1 Get failed: %v", err)
	}
	if string(retrievedValue1) != "value1" {
		t.Errorf("Scope1: expected value1, got %q", retrievedValue1)
	}

	retrievedValue2, err := scope2.Get(key)
	if err != nil {
		t.Fatalf("Scope2 Get failed: %v", err)
	}
	if string(retrievedValue2) != "value2" {
		t.Errorf("Scope2: expected value2, got %q", retrievedValue2)
	}
}

func TestVKV_VirtualScope_NestedScopes(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	level1 := vkv.NewVirtualScope([]byte("level1/"))
	level2 := level1.NewVirtualScope([]byte("level2/"))

	watcher := newMockWatcher()
	level2.AddWatcher(1, watcher)

	key := []byte("testkey")
	value := []byte("testvalue")

	_, err := level2.Set(key, value)
	if err != nil {
		t.Fatalf("Level2 Set failed: %v", err)
	}

	waitForWatcherNotifications(t, 100*time.Millisecond)

	calls := watcher.getOnSetCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 OnSet call, got %d", len(calls))
	}

	call := calls[0]
	if string(call.Key) != "testkey" {
		t.Errorf("Expected unscoped key in watcher call, got %q", call.Key)
	}

	retrievedValue, err := level2.Get(key)
	if err != nil {
		t.Fatalf("Level2 Get failed: %v", err)
	}
	if string(retrievedValue) != "testvalue" {
		t.Errorf("Expected %q, got %q", "testvalue", retrievedValue)
	}
}

func TestVKV_ConcurrentWatcherAccess(t *testing.T) {
	vkv, _, _, cleanup := setupVKVTest(t)
	defer cleanup()

	watcher := newMockWatcher()
	vkv.AddWatcher(1, watcher)

	const numGoroutines = 10
	const numOperationsPerGoroutine = 20

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperationsPerGoroutine; j++ {
				key := []byte("key-" + string(rune('0'+id)) + "-" + string(rune('0'+j)))
				value := []byte("value-" + string(rune('0'+id)) + "-" + string(rune('0'+j)))

				_, err := vkv.Set(key, value)
				if err != nil {
					t.Errorf("Set failed: %v", err)
				}
			}
		}(i)
	}

	wg.Wait()
	waitForWatcherNotifications(t, 200*time.Millisecond)

	calls := watcher.getOnSetCalls()
	expectedCalls := numGoroutines * numOperationsPerGoroutine
	if len(calls) != expectedCalls {
		t.Errorf("Expected %d OnSet calls, got %d", expectedCalls, len(calls))
	}
}
