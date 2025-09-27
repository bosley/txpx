package planar

import (
	"context"
	"sync"

	"github.com/bosley/txpx/pkg/pool"
)

type KeyWatcher interface {
	OnSet(key []byte)
	OnDelete(key []byte)
}

type VirtualKV interface {
	KV
	NewVirtualScope(scopePrefix []byte) VirtualKV

	AddWatcher(id uint8, watcher KeyWatcher)
	RemoveWatcher(id uint8)

	GetLockedVariant() LockedVirtualKV
}

type LockedVirtualKV interface {
	KV
	// no watchers, no adding scopes. just usage.
}

type vkv struct {
	watchPool         *pool.Pool
	kv                KV
	scopePrefix       []byte
	scopePrefixLength int

	watcherMu sync.Mutex
	watchers  map[uint8]KeyWatcher
}

func NewVirtualKV(watchPool *pool.Pool, kv KV, scopePrefix []byte) *vkv {
	return &vkv{
		watchPool:         watchPool,
		kv:                kv,
		scopePrefix:       scopePrefix,
		scopePrefixLength: len(scopePrefix),
		watchers:          make(map[uint8]KeyWatcher),
		watcherMu:         sync.Mutex{},
	}
}

var _ VirtualKV = &vkv{}

func (v *vkv) addScopePrefix(key []byte) []byte {
	result := make([]byte, v.scopePrefixLength+len(key))
	copy(result, v.scopePrefix)
	copy(result[v.scopePrefixLength:], key)
	return result
}

func (v *vkv) removeScopePrefix(key []byte) []byte {
	if len(key) < v.scopePrefixLength {
		return key
	}
	return key[v.scopePrefixLength:]
}

func (v *vkv) NewVirtualScope(scopePrefix []byte) VirtualKV {
	newPrefix := append(v.scopePrefix, scopePrefix...)
	return &vkv{
		watchPool:         v.watchPool,
		kv:                v.kv,
		scopePrefix:       newPrefix,
		scopePrefixLength: len(newPrefix),
		watchers:          make(map[uint8]KeyWatcher),
		watcherMu:         sync.Mutex{},
	}
}

func (v *vkv) GetLockedVariant() LockedVirtualKV {
	// Return itself but slice off the the other methods related to scopes and watchers
	return v
}

func (v *vkv) AddWatcher(id uint8, watcher KeyWatcher) {
	v.watcherMu.Lock()
	defer v.watcherMu.Unlock()
	v.watchers[id] = watcher
}

func (v *vkv) RemoveWatcher(id uint8) {
	v.watcherMu.Lock()
	defer v.watcherMu.Unlock()
	delete(v.watchers, id)
}

func (v *vkv) internalNotifyWatchersOnSet(key []byte) {
	v.watcherMu.Lock()
	defer v.watcherMu.Unlock()

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	for _, watcher := range v.watchers {
		v.watchPool.Submit(func(ctx context.Context) error {
			watcher.OnSet(keyCopy)
			return nil
		})
	}
}

func (v *vkv) internalNotifyWatchersOnDelete(key []byte) {
	v.watcherMu.Lock()
	defer v.watcherMu.Unlock()

	keyCopy := make([]byte, len(key))
	copy(keyCopy, key)
	for _, watcher := range v.watchers {
		v.watchPool.Submit(func(ctx context.Context) error {
			watcher.OnDelete(keyCopy)
			return nil
		})
	}
}

func (v *vkv) internalNotifyWatchersOnSetBatch(items []KVItem) {

	keys := make([][]byte, len(items))
	for i, item := range items {
		keys[i] = make([]byte, len(item.Key))
		copy(keys[i], item.Key)
	}

	v.watcherMu.Lock()
	defer v.watcherMu.Unlock()

	for _, watcher := range v.watchers {
		for _, key := range keys {
			v.watchPool.Submit(func(ctx context.Context) error {
				watcher.OnSet(key)
				return nil
			})
		}
	}
}

func (v *vkv) internalNotifyWatchersOnDeleteBatch(items []KVItem) {

	keys := make([][]byte, len(items))
	for i, item := range items {
		keys[i] = make([]byte, len(item.Key))
		copy(keys[i], item.Key)
	}

	v.watcherMu.Lock()
	defer v.watcherMu.Unlock()

	for _, watcher := range v.watchers {
		for _, key := range keys {
			v.watchPool.Submit(func(ctx context.Context) error {
				watcher.OnDelete(key)
				return nil
			})
		}
	}
}

// primitive kv

func (v *vkv) Get(key []byte) ([]byte, error) {
	return v.kv.Get(v.addScopePrefix(key))
}

func (v *vkv) Set(key []byte, value []byte) (KVMutationResult, error) {
	scopedKey := v.addScopePrefix(key)
	result, err := v.kv.Set(scopedKey, value)
	if err != nil {
		return result, err
	}
	go v.internalNotifyWatchersOnSet(key)
	return result, nil
}

func (v *vkv) Delete(key []byte) (KVMutationResult, error) {
	scopedKey := v.addScopePrefix(key)
	result, err := v.kv.Delete(scopedKey)
	if err != nil {
		return result, err
	}
	if result.UniqueKeyBytesDelta != 0 {
		go v.internalNotifyWatchersOnDelete(key)
	}
	return result, nil
}

func (v *vkv) Exists(key []byte) (bool, error) {
	return v.kv.Exists(v.addScopePrefix(key))
}

func (v *vkv) IterateKeys(prefix []byte, offset uint32, limit uint32, recv KVIPIterRecv) error {
	iterMiddleware := func(key []byte) (bool, error) {
		// Since we are scoping them we dont comply with the options to remove preifx. we always
		// remove OUR prefix from the key for isolation
		return recv(v.removeScopePrefix(key))
	}

	return v.kv.IterateKeys(v.addScopePrefix(prefix), offset, limit, iterMiddleware)
}

func (v *vkv) SetBatch(items []KVItem) (KVBatchMutationResult, error) {
	scopedItems := make([]KVItem, len(items))
	for i, item := range items {
		scopedItems[i] = KVItem{
			Key:   v.addScopePrefix(item.Key),
			Value: item.Value,
		}
	}
	result, err := v.kv.SetBatch(scopedItems)
	if err != nil {
		return result, err
	}
	go v.internalNotifyWatchersOnSetBatch(items)
	return result, nil
}

func (v *vkv) DeleteBatch(items []KVItem) (KVBatchMutationResult, error) {
	scopedItems := make([]KVItem, len(items))
	for i, item := range items {
		scopedItems[i] = KVItem{
			Key:   v.addScopePrefix(item.Key),
			Value: item.Value,
		}
	}
	result, err := v.kv.DeleteBatch(scopedItems)
	if err != nil {
		return result, err
	}
	go v.internalNotifyWatchersOnDeleteBatch(items)
	return result, nil
}

func (v *vkv) CompareAndSwap(key []byte, oldValue []byte, newValue []byte) (KVMutationResult, error) {
	scopedKey := v.addScopePrefix(key)
	result, err := v.kv.CompareAndSwap(scopedKey, oldValue, newValue)
	if err != nil {
		return result, err
	}

	go v.internalNotifyWatchersOnSet(key)
	return result, nil
}

func (v *vkv) CompareAndDelete(key []byte, oldValue []byte) (KVMutationResult, error) {
	scopedKey := v.addScopePrefix(key)
	result, err := v.kv.CompareAndDelete(scopedKey, oldValue)
	if err != nil {
		return result, err
	}

	// If key deleted, notify watchers
	if result.UniqueKeyBytesDelta != 0 {
		go v.internalNotifyWatchersOnDelete(key)
	}
	return result, nil
}

func (v *vkv) SetIfNotExists(key []byte, value []byte) (KVMutationResult, error) {
	scopedKey := v.addScopePrefix(key)
	result, err := v.kv.SetIfNotExists(scopedKey, value)
	if err != nil {
		return result, err
	}

	// If key set, notify watchers
	if result.UniqueKeyBytesDelta != 0 {
		go v.internalNotifyWatchersOnSet(key)
	}
	return result, nil
}

func (v *vkv) IterateItems(options KVIterOptions, recv KVIterRecv) error {
	scopedOptions := options
	scopedOptions.Prefix = v.addScopePrefix(options.Prefix)

	iterMiddleware := func(ctx context.Context, item KVItem) (bool, error) {
		item.Key = v.removeScopePrefix(item.Key)
		return recv(ctx, item)
	}
	return v.kv.IterateItems(scopedOptions, iterMiddleware)
}
