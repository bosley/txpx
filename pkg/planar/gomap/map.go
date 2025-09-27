package gomap

import (
	"bytes"
	"errors"
	"sort"
	"sync"

	"github.com/bosley/txpx/pkg/planar"
)

type mapBackend struct {
	mu   sync.RWMutex
	data map[string][]byte
}

var _ planar.KV = &mapBackend{}

func newMapKV() (planar.KV, error) {
	return &mapBackend{
		data: make(map[string][]byte),
	}, nil
}

func (m *mapBackend) Get(key []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("key cannot be empty")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	value, exists := m.data[string(key)]
	if !exists {
		return nil, errors.New("key not found")
	}

	result := make([]byte, len(value))
	copy(result, value)
	return result, nil
}

func (m *mapBackend) Set(key []byte, value []byte) (planar.KVMutationResult, error) {
	if len(key) == 0 {
		return planar.KVMutationResult{}, errors.New("key cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result planar.KVMutationResult
	keyStr := string(key)

	oldValue, exists := m.data[keyStr]
	if !exists {
		result.UniqueKeyBytesDelta = int64(len(key))
		result.UniqueValueBytesDelta = int64(len(value))
	} else {
		result.UniqueKeyBytesDelta = 0
		result.UniqueValueBytesDelta = int64(len(value)) - int64(len(oldValue))
	}

	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	m.data[keyStr] = valueCopy

	return result, nil
}

func (m *mapBackend) Delete(key []byte) (planar.KVMutationResult, error) {
	if len(key) == 0 {
		return planar.KVMutationResult{}, errors.New("key cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result planar.KVMutationResult
	keyStr := string(key)

	oldValue, exists := m.data[keyStr]
	if exists {
		result.UniqueKeyBytesDelta = -int64(len(key))
		result.UniqueValueBytesDelta = -int64(len(oldValue))
		delete(m.data, keyStr)
	}

	return result, nil
}

func (m *mapBackend) Exists(key []byte) (bool, error) {
	if len(key) == 0 {
		return false, errors.New("key cannot be empty")
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	_, exists := m.data[string(key)]
	return exists, nil
}

func (m *mapBackend) IterateKeys(prefix []byte, offset uint32, limit uint32, recv planar.KVIPIterRecv) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string

	for k := range m.data {
		if bytes.HasPrefix([]byte(k), prefix) {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	var count uint32
	for i := offset; i < uint32(len(keys)); i++ {
		if limit > 0 && count >= limit {
			break
		}

		cont, err := recv([]byte(keys[i]))
		if err != nil {
			return err
		}
		if !cont {
			break
		}
		count++
	}

	return nil
}

func (m *mapBackend) SetBatch(items []planar.KVItem) (planar.KVBatchMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result planar.KVBatchMutationResult
	originalState := make(map[string][]byte)
	keyExisted := make(map[string]bool)
	keyAlreadyCountedInBatch := make(map[string]bool)

	for _, item := range items {
		keyStr := string(item.Key)
		if _, captured := keyExisted[keyStr]; !captured {
			if value, exists := m.data[keyStr]; exists {
				originalState[keyStr] = value
				keyExisted[keyStr] = true
			} else {
				keyExisted[keyStr] = false
			}
		}
	}

	for _, item := range items {
		if len(item.Key) == 0 {
			return result, errors.New("key cannot be empty")
		}

		keyStr := string(item.Key)
		keyExistedBefore := keyExisted[keyStr]

		if !keyExistedBefore && !keyAlreadyCountedInBatch[keyStr] {
			result.UniqueKeyBytesDelta += int64(len(item.Key))
			result.UniqueValueBytesDelta += int64(len(item.Value))
			keyAlreadyCountedInBatch[keyStr] = true
		} else if !keyExistedBefore {
			result.UniqueValueBytesDelta += int64(len(item.Value))
		} else {
			oldValue := originalState[keyStr]
			result.UniqueValueBytesDelta += int64(len(item.Value)) - int64(len(oldValue))
		}

		valueCopy := make([]byte, len(item.Value))
		copy(valueCopy, item.Value)
		m.data[keyStr] = valueCopy
		result.KeysUpdated++
	}

	return result, nil
}

func (m *mapBackend) DeleteBatch(items []planar.KVItem) (planar.KVBatchMutationResult, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result planar.KVBatchMutationResult

	for _, item := range items {
		if len(item.Key) == 0 {
			return result, errors.New("key cannot be empty")
		}

		keyStr := string(item.Key)
		oldValue, exists := m.data[keyStr]

		if exists {
			result.UniqueKeyBytesDelta -= int64(len(item.Key))
			result.UniqueValueBytesDelta -= int64(len(oldValue))
			delete(m.data, keyStr)
			result.KeysUpdated++
		}
	}

	return result, nil
}

func (m *mapBackend) CompareAndSwap(key []byte, oldValue []byte, newValue []byte) (planar.KVMutationResult, error) {
	if len(key) == 0 {
		return planar.KVMutationResult{}, errors.New("key cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result planar.KVMutationResult
	keyStr := string(key)

	currentValue, exists := m.data[keyStr]
	if !exists {
		return result, errors.New("key not found")
	}

	if !bytes.Equal(currentValue, oldValue) {
		return result, planar.ErrKVAtomicFailureSwapValueMismatch
	}

	valueCopy := make([]byte, len(newValue))
	copy(valueCopy, newValue)
	m.data[keyStr] = valueCopy

	result.UniqueKeyBytesDelta = 0
	result.UniqueValueBytesDelta = int64(len(newValue)) - int64(len(oldValue))

	return result, nil
}

func (m *mapBackend) CompareAndDelete(key []byte, oldValue []byte) (planar.KVMutationResult, error) {
	if len(key) == 0 {
		return planar.KVMutationResult{}, errors.New("key cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result planar.KVMutationResult
	keyStr := string(key)

	currentValue, exists := m.data[keyStr]
	if !exists {
		return result, errors.New("key not found")
	}

	if !bytes.Equal(currentValue, oldValue) {
		return result, planar.ErrKVAtomicFailureSwapValueMismatch
	}

	delete(m.data, keyStr)
	result.UniqueKeyBytesDelta = -int64(len(key))
	result.UniqueValueBytesDelta = -int64(len(oldValue))

	return result, nil
}

func (m *mapBackend) SetIfNotExists(key []byte, value []byte) (planar.KVMutationResult, error) {
	if len(key) == 0 {
		return planar.KVMutationResult{}, errors.New("key cannot be empty")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	var result planar.KVMutationResult
	keyStr := string(key)

	_, exists := m.data[keyStr]
	if exists {
		return result, planar.ErrKVAtomicFailureKeyAlreadyExists
	}

	valueCopy := make([]byte, len(value))
	copy(valueCopy, value)
	m.data[keyStr] = valueCopy

	result.UniqueKeyBytesDelta = int64(len(key))
	result.UniqueValueBytesDelta = int64(len(value))

	return result, nil
}

func (m *mapBackend) IterateItems(options planar.KVIterOptions, recv planar.KVIterRecv) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var keys []string

	for k := range m.data {
		if bytes.HasPrefix([]byte(k), options.Prefix) {
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)

	for _, k := range keys {
		if options.Ctx != nil {
			select {
			case <-options.Ctx.Done():
				return options.Ctx.Err()
			default:
			}
		}

		key := []byte(k)
		if options.RemovePrefixOnCallback && len(options.Prefix) > 0 {
			key = key[len(options.Prefix):]
		}

		valueCopy := make([]byte, len(m.data[k]))
		copy(valueCopy, m.data[k])

		item := planar.KVItem{
			Key:   key,
			Value: valueCopy,
		}

		cont, err := recv(options.Ctx, item)
		if err != nil {
			return err
		}
		if !cont {
			break
		}
	}

	return nil
}
