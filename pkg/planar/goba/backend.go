package goba

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bosley/txpx/pkg/planar"

	"github.com/dgraph-io/badger/v3"
)

type BadgerBackend struct {
	mu     sync.Mutex
	dbs    map[string]*badger.DB
	logger *slog.Logger

	path string
}

func OpenBadgerBackend(path string) planar.KVProvider {
	if path == "" {
		panic("path cannot be empty")
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		if err := os.MkdirAll(path, 0755); err != nil {
			panic(err)
		}
		time.Sleep(1 * time.Second)
	}
	stat, err := os.Stat(path)
	if err != nil {
		panic(err)
	}
	if !stat.IsDir() {
		panic("path is not a directory")
	}
	return &BadgerBackend{
		dbs:  make(map[string]*badger.DB),
		path: path,
	}
}

func normalizeName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		switch r {
		case '/', '\\', '.', ' ', ':':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (b *BadgerBackend) LoadOrCreate(logger *slog.Logger, name string) (planar.KV, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.logger == nil {
		b.logger = logger.WithGroup("badger-backend")
	}

	targetPath := filepath.Join(b.path, normalizeName(name))

	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		if err := os.MkdirAll(targetPath, 0755); err != nil {
			return nil, err
		}
		time.Sleep(1 * time.Second)
	}

	// Check if we already have a DB for this path
	if db, exists := b.dbs[targetPath]; exists {
		b.logger.Warn("backend already exists for path, returning existing", "path", targetPath)
		return &badgerBackend{db: db}, nil
	}

	// Create new KV store
	kv, err := newBadgerKV(targetPath)
	if err != nil {
		return nil, err
	}

	// Store reference for cleanup
	if backend, ok := kv.(*badgerBackend); ok {
		b.dbs[targetPath] = backend.db
	}

	return kv, nil
}

func (b *BadgerBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	var firstErr error
	for path, db := range b.dbs {
		if err := db.Close(); err != nil {
			b.logger.Error("failed to close badger db", "path", path, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		} else {
			b.logger.Info("closed badger db", "path", path)
		}
		delete(b.dbs, path)
	}

	return firstErr
}
