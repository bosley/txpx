package gomap

import (
	"log/slog"
	"sync"

	"github.com/bosley/txpx/pkg/planar"
)

// A more efficient use would be a badger with the "memory" flag, but i wanted a non-badger implementation for testing/examples.
type MapBackend struct {
	mu     sync.Mutex
	stores map[string]*mapBackend
	logger *slog.Logger

	name string
}

func OpenMapBackend(name string) planar.KVProvider {
	if name == "" {
		name = "default"
	}
	return &MapBackend{
		stores: make(map[string]*mapBackend),
		name:   name,
	}
}

func (b *MapBackend) LoadOrCreate(logger *slog.Logger, name string) (planar.KV, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.logger == nil {
		b.logger = logger.WithGroup("map-backend")
	}

	if store, exists := b.stores[b.name]; exists {
		b.logger.Warn("backend already exists for name, returning existing", "name", b.name)
		return store, nil
	}

	kv, err := newMapKV()
	if err != nil {
		return nil, err
	}

	if backend, ok := kv.(*mapBackend); ok {
		b.stores[b.name] = backend
	}

	return kv, nil
}

func (b *MapBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var firstErr error
	for name := range b.stores {
		b.logger.Info("closed map store", "name", name)
		delete(b.stores, name)
	}

	return firstErr
}
