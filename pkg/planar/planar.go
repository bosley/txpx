package planar

import (
	"context"
	"errors"
	"log/slog"
)

var (
	ErrKVKeyNotFound                    = errors.New("key not found")
	ErrKVAtomicFailureKeyAlreadyExists  = errors.New("key already exists")
	ErrKVAtomicFailureSwapValueMismatch = errors.New("expectedvalue mismatch")
)

type KVProvider interface {
	LoadOrCreate(logger *slog.Logger, name string) (KV, error)
	Close() error
}

// This result type respresents the change in bytes on disk/memory after a single mutation of
// a primitive operation on a key-value concept.
// set - if an item existed, the value delta must be the difference between the new and old value (> 0 if new is larger, < 0 if new is smaller)
//
//	In practice the key bytes delta on a set should always be 0 unless its a unique key
//
// delete - if item existed, the key and value delta must always be < 0 as its a full deletion of all the bytes for that entry
type KVMutationResult struct {
	UniqueKeyBytesDelta   int64
	UniqueValueBytesDelta int64
}

// bool - continue iteration, error - there was an error (wont stop unless explicitly asked to stop)
type KVIPIterRecv func(key []byte) (bool, error)

// This is the most primitive interface we can have, we build the usable, atomic one using this.
type KVIPrimitive interface {
	Get(key []byte) ([]byte, error)
	Set(key []byte, value []byte) (KVMutationResult, error)
	Delete(key []byte) (KVMutationResult, error)

	Exists(key []byte) (bool, error) // bool cant be trusted if error is non-nil

	IterateKeys(prefix []byte, offset uint32, limit uint32, recv KVIPIterRecv) error
}

// From KVIP we can build the following:

type KVItem struct {
	Key   []byte
	Value []byte
}

type KVBatchMutationResult struct {
	KeysUpdated           uint64
	UniqueKeyBytesDelta   int64
	UniqueValueBytesDelta int64
}

type KVIterRecv func(ctx context.Context, item KVItem) (bool, error) // continue iteration, error - there was an error (wont stop unless explicitly asked to stop)

type KVIterOptions struct {
	Ctx                    context.Context
	Prefix                 []byte
	RemovePrefixOnCallback bool
}

type KV interface {
	KVIPrimitive

	SetBatch(items []KVItem) (KVBatchMutationResult, error)
	DeleteBatch(items []KVItem) (KVBatchMutationResult, error)

	CompareAndSwap(key []byte, oldValue []byte, newValue []byte) (KVMutationResult, error)
	CompareAndDelete(key []byte, oldValue []byte) (KVMutationResult, error)

	SetIfNotExists(key []byte, value []byte) (KVMutationResult, error)

	IterateItems(options KVIterOptions, recv KVIterRecv) error
}
