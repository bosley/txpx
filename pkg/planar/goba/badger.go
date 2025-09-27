package goba

import (
	"bytes"
	"errors"
	"os"

	"github.com/bosley/txpx/pkg/planar"

	"github.com/dgraph-io/badger/v3"
)

type badgerBackend struct {
	db *badger.DB
}

var _ planar.KV = &badgerBackend{}

func newBadgerKV(absolutePathOnDisk string) (planar.KV, error) {

	if _, err := os.Stat(absolutePathOnDisk); os.IsNotExist(err) {
		if err := os.MkdirAll(absolutePathOnDisk, 0755); err != nil {
			return nil, err
		}
	}

	db, err := badger.Open(badger.DefaultOptions(absolutePathOnDisk))
	if err != nil {
		return nil, err
	}

	return &badgerBackend{
		db: db,
	}, nil
}

func (b *badgerBackend) Get(key []byte) ([]byte, error) {
	var value []byte
	err := b.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return planar.ErrKVKeyNotFound
			}
			return err
		}
		value, err = item.ValueCopy(nil)
		return err
	})
	return value, err
}

func (b *badgerBackend) Set(key []byte, value []byte) (planar.KVMutationResult, error) {
	var result planar.KVMutationResult
	var oldValueSize int64

	err := b.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		keyExists := err == nil
		if keyExists {
			oldValueSize = item.ValueSize()
		} else if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		if err := txn.Set(key, value); err != nil {
			return err
		}

		if !keyExists {
			result.UniqueKeyBytesDelta = int64(len(key))
			result.UniqueValueBytesDelta = int64(len(value))
		} else {
			result.UniqueKeyBytesDelta = 0
			result.UniqueValueBytesDelta = int64(len(value)) - oldValueSize
		}

		return nil
	})

	return result, err
}

func (b *badgerBackend) Delete(key []byte) (planar.KVMutationResult, error) {
	var result planar.KVMutationResult

	err := b.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			if errors.Is(err, badger.ErrKeyNotFound) {
				return nil
			}
			return err
		}

		valueSize := item.ValueSize()

		if err := txn.Delete(key); err != nil {
			return err
		}

		result.UniqueKeyBytesDelta = -int64(len(key))
		result.UniqueValueBytesDelta = -valueSize

		return nil
	})

	return result, err
}

func (b *badgerBackend) Exists(key []byte) (bool, error) {
	var exists bool
	err := b.db.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err == nil {
			exists = true
			return nil
		}
		if errors.Is(err, badger.ErrKeyNotFound) {
			exists = false
			return nil
		}
		return err
	})
	return exists, err
}

func (b *badgerBackend) IterateKeys(prefix []byte, offset uint32, limit uint32, recv planar.KVIPIterRecv) error {
	return b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		opts.Prefix = prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		var count uint32
		var skipped uint32

		for it.Rewind(); it.Valid(); it.Next() {
			if skipped < offset {
				skipped++
				continue
			}

			if limit > 0 && count >= limit {
				break
			}

			key := it.Item().KeyCopy(nil)
			cont, err := recv(key)
			if err != nil {
				return err
			}
			if !cont {
				break
			}

			count++
		}

		return nil
	})
}

func (b *badgerBackend) SetBatch(items []planar.KVItem) (planar.KVBatchMutationResult, error) {
	var result planar.KVBatchMutationResult

	err := b.db.Update(func(txn *badger.Txn) error {
		for _, item := range items {
			oldItem, err := txn.Get(item.Key)
			var oldValueSize int64
			var keyExists bool

			if err == nil {
				oldValueSize = oldItem.ValueSize()
				keyExists = true
			} else if !errors.Is(err, badger.ErrKeyNotFound) {
				return err
			}

			if err := txn.Set(item.Key, item.Value); err != nil {
				return err
			}

			result.KeysUpdated++

			if !keyExists {
				result.UniqueKeyBytesDelta += int64(len(item.Key))
				result.UniqueValueBytesDelta += int64(len(item.Value))
			} else {
				result.UniqueValueBytesDelta += int64(len(item.Value)) - oldValueSize
			}
		}
		return nil
	})

	return result, err
}

func (b *badgerBackend) DeleteBatch(items []planar.KVItem) (planar.KVBatchMutationResult, error) {
	var result planar.KVBatchMutationResult

	err := b.db.Update(func(txn *badger.Txn) error {
		for _, item := range items {
			oldItem, err := txn.Get(item.Key)
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue
				}
				return err
			}

			valueSize := oldItem.ValueSize()

			if err := txn.Delete(item.Key); err != nil {
				return err
			}

			result.KeysUpdated++
			result.UniqueKeyBytesDelta -= int64(len(item.Key))
			result.UniqueValueBytesDelta -= valueSize
		}
		return nil
	})

	return result, err
}

func (b *badgerBackend) CompareAndSwap(key []byte, oldValue []byte, newValue []byte) (planar.KVMutationResult, error) {
	var result planar.KVMutationResult

	err := b.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		currentValue, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		if !bytes.Equal(currentValue, oldValue) {
			return planar.ErrKVAtomicFailureSwapValueMismatch
		}

		if err := txn.Set(key, newValue); err != nil {
			return err
		}

		result.UniqueKeyBytesDelta = 0
		result.UniqueValueBytesDelta = int64(len(newValue)) - int64(len(oldValue))

		return nil
	})

	return result, err
}

func (b *badgerBackend) CompareAndDelete(key []byte, oldValue []byte) (planar.KVMutationResult, error) {
	var result planar.KVMutationResult

	err := b.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}

		currentValue, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}

		if !bytes.Equal(currentValue, oldValue) {
			return planar.ErrKVAtomicFailureSwapValueMismatch
		}

		if err := txn.Delete(key); err != nil {
			return err
		}

		result.UniqueKeyBytesDelta = -int64(len(key))
		result.UniqueValueBytesDelta = -int64(len(oldValue))

		return nil
	})

	return result, err
}

func (b *badgerBackend) SetIfNotExists(key []byte, value []byte) (planar.KVMutationResult, error) {
	var result planar.KVMutationResult

	err := b.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		if err == nil {
			return planar.ErrKVAtomicFailureKeyAlreadyExists
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		if err := txn.Set(key, value); err != nil {
			return err
		}

		result.UniqueKeyBytesDelta = int64(len(key))
		result.UniqueValueBytesDelta = int64(len(value))

		return nil
	})

	return result, err
}

func (b *badgerBackend) IterateItems(options planar.KVIterOptions, recv planar.KVIterRecv) error {
	return b.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.Prefix = options.Prefix

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			if options.Ctx != nil {
				select {
				case <-options.Ctx.Done():
					return options.Ctx.Err()
				default:
				}
			}

			item := it.Item()
			key := item.KeyCopy(nil)
			value, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			if options.RemovePrefixOnCallback && len(options.Prefix) > 0 {
				key = key[len(options.Prefix):]
			}

			kvItem := planar.KVItem{
				Key:   key,
				Value: value,
			}

			cont, err := recv(options.Ctx, kvItem)
			if err != nil {
				return err
			}
			if !cont {
				break
			}
		}

		return nil
	})
}
