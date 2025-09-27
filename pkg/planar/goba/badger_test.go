package goba

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bosley/txpx/pkg/planar"
)

func TestKVIPrimitive_Set(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name               string
		key                []byte
		value              []byte
		expectedKeyDelta   int64
		expectedValueDelta int64
		setupExistingValue []byte
	}{
		{
			name:               "set new key",
			key:                []byte("key1"),
			value:              []byte("value1"),
			expectedKeyDelta:   4,
			expectedValueDelta: 6,
		},
		{
			name:               "update existing key with larger value",
			key:                []byte("key2"),
			value:              []byte("largervalue"),
			setupExistingValue: []byte("small"),
			expectedKeyDelta:   0,
			expectedValueDelta: 6,
		},
		{
			name:               "update existing key with smaller value",
			key:                []byte("key3"),
			value:              []byte("tiny"),
			setupExistingValue: []byte("hugevalue"),
			expectedKeyDelta:   0,
			expectedValueDelta: -5,
		},
		{
			name:               "update existing key with same size value",
			key:                []byte("key4"),
			value:              []byte("same"),
			setupExistingValue: []byte("size"),
			expectedKeyDelta:   0,
			expectedValueDelta: 0,
		},
		{
			name:               "set empty value on new key",
			key:                []byte("key5"),
			value:              []byte(""),
			expectedKeyDelta:   4,
			expectedValueDelta: 0,
		},
		{
			name:               "update to empty value",
			key:                []byte("key6"),
			value:              []byte(""),
			setupExistingValue: []byte("something"),
			expectedKeyDelta:   0,
			expectedValueDelta: -9,
		},
		{
			name:               "set large key and value",
			key:                make([]byte, 1000),
			value:              make([]byte, 5000),
			expectedKeyDelta:   1000,
			expectedValueDelta: 5000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupExistingValue != nil {
				_, err := db.Set(tt.key, tt.setupExistingValue)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			result, err := db.Set(tt.key, tt.value)
			if err != nil {
				t.Fatalf("Set failed: %v", err)
			}

			if result.UniqueKeyBytesDelta != tt.expectedKeyDelta {
				t.Errorf("key delta mismatch: got %d, want %d", result.UniqueKeyBytesDelta, tt.expectedKeyDelta)
			}
			if result.UniqueValueBytesDelta != tt.expectedValueDelta {
				t.Errorf("value delta mismatch: got %d, want %d", result.UniqueValueBytesDelta, tt.expectedValueDelta)
			}

			gotValue, err := db.Get(tt.key)
			if err != nil {
				t.Fatalf("Get after Set failed: %v", err)
			}
			if !bytes.Equal(gotValue, tt.value) {
				t.Errorf("stored value mismatch: got %q, want %q", gotValue, tt.value)
			}
		})
	}
}

func TestKVIPrimitive_Delete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name               string
		key                []byte
		setupValue         []byte
		expectedKeyDelta   int64
		expectedValueDelta int64
	}{
		{
			name:               "delete existing key",
			key:                []byte("key1"),
			setupValue:         []byte("value1"),
			expectedKeyDelta:   -4,
			expectedValueDelta: -6,
		},
		{
			name:               "delete non-existent key",
			key:                []byte("nokey"),
			expectedKeyDelta:   0,
			expectedValueDelta: 0,
		},
		{
			name:               "delete key with empty value",
			key:                []byte("emptykey"),
			setupValue:         []byte(""),
			expectedKeyDelta:   -8,
			expectedValueDelta: 0,
		},
		{
			name:               "delete key with large value",
			key:                []byte("k"),
			setupValue:         []byte("this is a very large value to test byte counting"),
			expectedKeyDelta:   -1,
			expectedValueDelta: -48,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupValue != nil {
				_, err := db.Set(tt.key, tt.setupValue)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			result, err := db.Delete(tt.key)
			if err != nil {
				t.Fatalf("Delete failed: %v", err)
			}

			if result.UniqueKeyBytesDelta != tt.expectedKeyDelta {
				t.Errorf("key delta mismatch: got %d, want %d", result.UniqueKeyBytesDelta, tt.expectedKeyDelta)
			}
			if result.UniqueValueBytesDelta != tt.expectedValueDelta {
				t.Errorf("value delta mismatch: got %d, want %d", result.UniqueValueBytesDelta, tt.expectedValueDelta)
			}

			exists, err := db.Exists(tt.key)
			if err != nil {
				t.Fatalf("Exists after Delete failed: %v", err)
			}
			if exists {
				t.Error("key should not exist after delete")
			}
		})
	}
}

func TestKVIPrimitive_Get(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	tests := []struct {
		name        string
		key         []byte
		setupValue  []byte
		expectError bool
	}{
		{
			name:        "get existing key",
			key:         []byte("testkey"),
			setupValue:  []byte("testvalue"),
			expectError: false,
		},
		{
			name:        "get non-existent key",
			key:         []byte("nonexistent"),
			expectError: true,
		},
		{
			name:        "get key with empty value",
			key:         []byte("emptyvalue"),
			setupValue:  []byte(""),
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupValue != nil {
				_, err := db.Set(tt.key, tt.setupValue)
				if err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			got, err := db.Get(tt.key)
			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("Get failed: %v", err)
				}
				if !bytes.Equal(got, tt.setupValue) {
					t.Errorf("Get returned wrong value: got %q, want %q", got, tt.setupValue)
				}
			}
		})
	}
}

func TestKVIPrimitive_Exists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	key := []byte("existkey")
	_, err := db.Set(key, []byte("value"))
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	exists, err := db.Exists(key)
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("Exists returned false for existing key")
	}

	exists, err = db.Exists([]byte("nokey"))
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("Exists returned true for non-existent key")
	}
}

func TestKVIPrimitive_IterateKeys(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	keys := [][]byte{
		[]byte("prefix/a"),
		[]byte("prefix/b"),
		[]byte("prefix/c"),
		[]byte("other/d"),
	}
	for _, k := range keys {
		_, err := db.Set(k, []byte("value"))
		if err != nil {
			t.Fatalf("Set failed: %v", err)
		}
	}

	t.Run("iterate all with prefix", func(t *testing.T) {
		var collected [][]byte
		err := db.IterateKeys([]byte("prefix/"), 0, 0, func(key []byte) (bool, error) {
			collected = append(collected, append([]byte(nil), key...))
			return true, nil
		})
		if err != nil {
			t.Fatalf("IterateKeys failed: %v", err)
		}
		if len(collected) != 3 {
			t.Errorf("expected 3 keys, got %d", len(collected))
		}
	})

	t.Run("iterate with offset and limit", func(t *testing.T) {
		var collected [][]byte
		err := db.IterateKeys([]byte("prefix/"), 1, 1, func(key []byte) (bool, error) {
			collected = append(collected, append([]byte(nil), key...))
			return true, nil
		})
		if err != nil {
			t.Fatalf("IterateKeys with offset/limit failed: %v", err)
		}
		if len(collected) != 1 {
			t.Errorf("expected 1 key with offset=1 limit=1, got %d", len(collected))
		}
	})

	t.Run("stop iteration early", func(t *testing.T) {
		var count int
		err := db.IterateKeys([]byte("prefix/"), 0, 0, func(key []byte) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("IterateKeys failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected iteration to stop after 1 key, got %d", count)
		}
	})

	t.Run("error during iteration", func(t *testing.T) {
		expectedErr := errors.New("test error")
		err := db.IterateKeys([]byte("prefix/"), 0, 0, func(key []byte) (bool, error) {
			return false, expectedErr
		})
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("empty prefix", func(t *testing.T) {
		var collected [][]byte
		err := db.IterateKeys(nil, 0, 0, func(key []byte) (bool, error) {
			collected = append(collected, append([]byte(nil), key...))
			return true, nil
		})
		if err != nil {
			t.Fatalf("IterateKeys failed: %v", err)
		}
		if len(collected) != 4 {
			t.Errorf("expected 4 keys with empty prefix, got %d", len(collected))
		}
	})
}

func TestKV_SetBatch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	_, err := db.Set([]byte("existing"), []byte("old"))
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	tests := []struct {
		name               string
		items              []planar.KVItem
		expectedKeyDelta   int64
		expectedValueDelta int64
		expectedKeys       uint64
	}{
		{
			name: "all new keys",
			items: []planar.KVItem{
				{Key: []byte("new1"), Value: []byte("value1")},
				{Key: []byte("new2"), Value: []byte("value22")},
			},
			expectedKeyDelta:   8,
			expectedValueDelta: 13,
			expectedKeys:       2,
		},
		{
			name: "mix of new and existing",
			items: []planar.KVItem{
				{Key: []byte("new3"), Value: []byte("value3")},
				{Key: []byte("existing"), Value: []byte("updated")},
			},
			expectedKeyDelta:   4,
			expectedValueDelta: 10,
			expectedKeys:       2,
		},
		{
			name:               "empty batch",
			items:              []planar.KVItem{},
			expectedKeyDelta:   0,
			expectedValueDelta: 0,
			expectedKeys:       0,
		},
		{
			name: "duplicate keys in batch",
			items: []planar.KVItem{
				{Key: []byte("dup"), Value: []byte("first")},
				{Key: []byte("dup"), Value: []byte("second")},
			},
			expectedKeyDelta:   3,
			expectedValueDelta: 11,
			expectedKeys:       2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.SetBatch(tt.items)
			if err != nil {
				t.Fatalf("SetBatch failed: %v", err)
			}

			if result.KeysUpdated != tt.expectedKeys {
				t.Errorf("wrong keys updated: got %d, want %d", result.KeysUpdated, tt.expectedKeys)
			}
			if result.UniqueKeyBytesDelta != tt.expectedKeyDelta {
				t.Errorf("key delta mismatch: got %d, want %d", result.UniqueKeyBytesDelta, tt.expectedKeyDelta)
			}
			if result.UniqueValueBytesDelta != tt.expectedValueDelta {
				t.Errorf("value delta mismatch: got %d, want %d", result.UniqueValueBytesDelta, tt.expectedValueDelta)
			}

			for _, item := range tt.items {
				got, err := db.Get(item.Key)
				if err != nil {
					t.Errorf("failed to get key %q after batch: %v", item.Key, err)
					continue
				}
				if tt.name == "duplicate keys in batch" && bytes.Equal(item.Key, []byte("dup")) {
					if !bytes.Equal(got, []byte("second")) {
						t.Errorf("duplicate key should have last value: got %q, want %q", got, "second")
					}
				} else if !bytes.Equal(got, item.Value) {
					t.Errorf("value mismatch for key %q: got %q, want %q", item.Key, got, item.Value)
				}
			}
		})
	}
}

func TestKV_DeleteBatch(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	setupItems := []planar.KVItem{
		{Key: []byte("del1"), Value: []byte("value1")},
		{Key: []byte("del2"), Value: []byte("value22")},
		{Key: []byte("keep"), Value: []byte("keeper")},
	}
	for _, item := range setupItems {
		_, err := db.Set(item.Key, item.Value)
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	tests := []struct {
		name               string
		deleteItems        []planar.KVItem
		expectedKeyDelta   int64
		expectedValueDelta int64
		expectedKeys       uint64
	}{
		{
			name: "delete existing keys",
			deleteItems: []planar.KVItem{
				{Key: []byte("del1")},
				{Key: []byte("del2")},
			},
			expectedKeyDelta:   -8,
			expectedValueDelta: -13,
			expectedKeys:       2,
		},
		{
			name: "delete mix of existing and non-existent",
			deleteItems: []planar.KVItem{
				{Key: []byte("keep")},
				{Key: []byte("nonexistent")},
			},
			expectedKeyDelta:   -4,
			expectedValueDelta: -6,
			expectedKeys:       1,
		},
		{
			name:               "empty batch",
			deleteItems:        []planar.KVItem{},
			expectedKeyDelta:   0,
			expectedValueDelta: 0,
			expectedKeys:       0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := db.DeleteBatch(tt.deleteItems)
			if err != nil {
				t.Fatalf("DeleteBatch failed: %v", err)
			}

			if result.KeysUpdated != tt.expectedKeys {
				t.Errorf("wrong keys updated: got %d, want %d", result.KeysUpdated, tt.expectedKeys)
			}
			if result.UniqueKeyBytesDelta != tt.expectedKeyDelta {
				t.Errorf("key delta mismatch: got %d, want %d", result.UniqueKeyBytesDelta, tt.expectedKeyDelta)
			}
			if result.UniqueValueBytesDelta != tt.expectedValueDelta {
				t.Errorf("value delta mismatch: got %d, want %d", result.UniqueValueBytesDelta, tt.expectedValueDelta)
			}

			for _, item := range tt.deleteItems {
				exists, _ := db.Exists(item.Key)
				if exists && item.Key != nil {
					t.Errorf("key %q should not exist after delete", item.Key)
				}
			}
		})
	}
}

func TestKV_CompareAndSwap(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	key := []byte("caskey")
	oldValue := []byte("oldval")
	newValue := []byte("newvalue")

	_, err := db.Set(key, oldValue)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	t.Run("successful swap", func(t *testing.T) {
		result, err := db.CompareAndSwap(key, oldValue, newValue)
		if err != nil {
			t.Fatalf("CompareAndSwap failed: %v", err)
		}

		if result.UniqueKeyBytesDelta != 0 {
			t.Errorf("key delta should be 0 for update, got %d", result.UniqueKeyBytesDelta)
		}
		if result.UniqueValueBytesDelta != 2 {
			t.Errorf("value delta mismatch: got %d, want 2", result.UniqueValueBytesDelta)
		}

		got, _ := db.Get(key)
		if !bytes.Equal(got, newValue) {
			t.Errorf("value not updated: got %q, want %q", got, newValue)
		}
	})

	t.Run("value mismatch", func(t *testing.T) {
		_, err := db.CompareAndSwap(key, oldValue, []byte("fail"))
		if !errors.Is(err, planar.ErrKVAtomicFailureSwapValueMismatch) {
			t.Errorf("expected ErrKVAtomicFailureSwapValueMismatch, got %v", err)
		}
	})

	t.Run("non-existent key", func(t *testing.T) {
		_, err := db.CompareAndSwap([]byte("nokey"), []byte("old"), []byte("new"))
		if err == nil {
			t.Error("expected error for non-existent key")
		}
	})

	t.Run("swap to empty value", func(t *testing.T) {
		testKey := []byte("emptyswap")
		testOld := []byte("something")
		_, _ = db.Set(testKey, testOld)

		result, err := db.CompareAndSwap(testKey, testOld, []byte(""))
		if err != nil {
			t.Fatalf("CompareAndSwap to empty failed: %v", err)
		}
		if result.UniqueValueBytesDelta != -9 {
			t.Errorf("value delta mismatch: got %d, want -9", result.UniqueValueBytesDelta)
		}
	})
}

func TestKV_CompareAndDelete(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("successful delete", func(t *testing.T) {
		key := []byte("cadkey")
		value := []byte("delval")

		_, err := db.Set(key, value)
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}

		result, err := db.CompareAndDelete(key, value)
		if err != nil {
			t.Fatalf("CompareAndDelete failed: %v", err)
		}

		if result.UniqueKeyBytesDelta != -6 {
			t.Errorf("key delta mismatch: got %d, want -6", result.UniqueKeyBytesDelta)
		}
		if result.UniqueValueBytesDelta != -6 {
			t.Errorf("value delta mismatch: got %d, want -6", result.UniqueValueBytesDelta)
		}

		exists, _ := db.Exists(key)
		if exists {
			t.Error("key should not exist after CompareAndDelete")
		}
	})

	t.Run("value mismatch", func(t *testing.T) {
		key := []byte("mismatch")
		_, _ = db.Set(key, []byte("actual"))

		_, err := db.CompareAndDelete(key, []byte("wrong"))
		if !errors.Is(err, planar.ErrKVAtomicFailureSwapValueMismatch) {
			t.Errorf("expected ErrKVAtomicFailureSwapValueMismatch, got %v", err)
		}

		exists, _ := db.Exists(key)
		if !exists {
			t.Error("key should still exist after failed CompareAndDelete")
		}
	})

	t.Run("non-existent key", func(t *testing.T) {
		_, err := db.CompareAndDelete([]byte("nokey"), []byte("value"))
		if err == nil {
			t.Error("expected error for non-existent key")
		}
	})
}

func TestKV_SetIfNotExists(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("set new key", func(t *testing.T) {
		key := []byte("sinekey")
		value := []byte("newval")

		result, err := db.SetIfNotExists(key, value)
		if err != nil {
			t.Fatalf("SetIfNotExists failed: %v", err)
		}

		if result.UniqueKeyBytesDelta != 7 {
			t.Errorf("key delta mismatch: got %d, want 7", result.UniqueKeyBytesDelta)
		}
		if result.UniqueValueBytesDelta != 6 {
			t.Errorf("value delta mismatch: got %d, want 6", result.UniqueValueBytesDelta)
		}

		got, _ := db.Get(key)
		if !bytes.Equal(got, value) {
			t.Errorf("value mismatch: got %q, want %q", got, value)
		}
	})

	t.Run("key already exists", func(t *testing.T) {
		key := []byte("existing")
		_, _ = db.Set(key, []byte("already here"))

		_, err := db.SetIfNotExists(key, []byte("fail"))
		if !errors.Is(err, planar.ErrKVAtomicFailureKeyAlreadyExists) {
			t.Errorf("expected ErrKVAtomicFailureKeyAlreadyExists, got %v", err)
		}

		got, _ := db.Get(key)
		if !bytes.Equal(got, []byte("already here")) {
			t.Error("value should not have changed")
		}
	})

	t.Run("set empty value if not exists", func(t *testing.T) {
		key := []byte("emptyifnot")
		result, err := db.SetIfNotExists(key, []byte(""))
		if err != nil {
			t.Fatalf("SetIfNotExists with empty value failed: %v", err)
		}
		if result.UniqueKeyBytesDelta != 10 {
			t.Errorf("key delta mismatch: got %d, want 10", result.UniqueKeyBytesDelta)
		}
		if result.UniqueValueBytesDelta != 0 {
			t.Errorf("value delta mismatch: got %d, want 0", result.UniqueValueBytesDelta)
		}
	})
}

func TestKV_IterateItems(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	items := []planar.KVItem{
		{Key: []byte("iter/a"), Value: []byte("va")},
		{Key: []byte("iter/b"), Value: []byte("vb")},
		{Key: []byte("iter/c"), Value: []byte("vc")},
		{Key: []byte("other/d"), Value: []byte("vd")},
	}
	for _, item := range items {
		_, err := db.Set(item.Key, item.Value)
		if err != nil {
			t.Fatalf("setup failed: %v", err)
		}
	}

	t.Run("basic iteration", func(t *testing.T) {
		var collected []planar.KVItem
		err := db.IterateItems(planar.KVIterOptions{
			Prefix: []byte("iter/"),
		}, func(ctx context.Context, item planar.KVItem) (bool, error) {
			collected = append(collected, item)
			return true, nil
		})
		if err != nil {
			t.Fatalf("IterateItems failed: %v", err)
		}

		if len(collected) != 3 {
			t.Errorf("expected 3 items, got %d", len(collected))
		}

		for i, item := range collected {
			if !bytes.HasPrefix(item.Key, []byte("iter/")) {
				t.Errorf("item %d has wrong prefix: %q", i, item.Key)
			}
		}
	})

	t.Run("with prefix removal", func(t *testing.T) {
		var collected []planar.KVItem
		err := db.IterateItems(planar.KVIterOptions{
			Prefix:                 []byte("iter/"),
			RemovePrefixOnCallback: true,
		}, func(ctx context.Context, item planar.KVItem) (bool, error) {
			collected = append(collected, item)
			return true, nil
		})
		if err != nil {
			t.Fatalf("IterateItems with prefix removal failed: %v", err)
		}

		for i, item := range collected {
			if bytes.HasPrefix(item.Key, []byte("iter/")) {
				t.Errorf("item %d should have prefix removed: %q", i, item.Key)
			}
			if len(item.Key) != 1 {
				t.Errorf("item %d has wrong key length after prefix removal: %q", i, item.Key)
			}
		}
	})

	t.Run("with context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		var count int
		err := db.IterateItems(planar.KVIterOptions{
			Ctx:    ctx,
			Prefix: []byte("iter/"),
		}, func(ctx context.Context, item planar.KVItem) (bool, error) {
			count++
			return true, nil
		})

		if err == nil {
			t.Error("expected error with cancelled context")
		}
		if count > 0 {
			t.Error("should not iterate with already cancelled context")
		}
	})

	t.Run("stop iteration early", func(t *testing.T) {
		var count int
		err := db.IterateItems(planar.KVIterOptions{
			Prefix: []byte("iter/"),
		}, func(ctx context.Context, item planar.KVItem) (bool, error) {
			count++
			return false, nil
		})
		if err != nil {
			t.Fatalf("IterateItems failed: %v", err)
		}
		if count != 1 {
			t.Errorf("expected iteration to stop after 1 item, got %d", count)
		}
	})

	t.Run("error during iteration", func(t *testing.T) {
		expectedErr := errors.New("iteration error")
		err := db.IterateItems(planar.KVIterOptions{
			Prefix: []byte("iter/"),
		}, func(ctx context.Context, item planar.KVItem) (bool, error) {
			return false, expectedErr
		})
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected error %v, got %v", expectedErr, err)
		}
	})

	t.Run("empty prefix iterates all", func(t *testing.T) {
		var count int
		err := db.IterateItems(planar.KVIterOptions{}, func(ctx context.Context, item planar.KVItem) (bool, error) {
			count++
			return true, nil
		})
		if err != nil {
			t.Fatalf("IterateItems failed: %v", err)
		}
		if count != 4 {
			t.Errorf("expected 4 items with empty prefix, got %d", count)
		}
	})

	t.Run("context timeout during iteration", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		var count int
		err := db.IterateItems(planar.KVIterOptions{
			Ctx:    ctx,
			Prefix: []byte("iter/"),
		}, func(ctx context.Context, item planar.KVItem) (bool, error) {
			count++
			time.Sleep(30 * time.Millisecond)
			return true, nil
		})

		if err == nil && count >= 3 {
			t.Error("expected timeout error during iteration")
		}
	})
}

func TestByteDeltaAccuracy_ComplexScenario(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	var totalKeyDelta, totalValueDelta int64

	result1, _ := db.Set([]byte("k1"), []byte("initial"))
	totalKeyDelta += result1.UniqueKeyBytesDelta
	totalValueDelta += result1.UniqueValueBytesDelta

	result2, _ := db.Set([]byte("k1"), []byte("updated-longer"))
	totalKeyDelta += result2.UniqueKeyBytesDelta
	totalValueDelta += result2.UniqueValueBytesDelta

	batchResult1, _ := db.SetBatch([]planar.KVItem{
		{Key: []byte("k2"), Value: []byte("v2")},
		{Key: []byte("k3"), Value: []byte("v3")},
	})
	totalKeyDelta += batchResult1.UniqueKeyBytesDelta
	totalValueDelta += batchResult1.UniqueValueBytesDelta

	result3, _ := db.Delete([]byte("k2"))
	totalKeyDelta += result3.UniqueKeyBytesDelta
	totalValueDelta += result3.UniqueValueBytesDelta

	result4, _ := db.Set([]byte("k1"), []byte("tiny"))
	totalKeyDelta += result4.UniqueKeyBytesDelta
	totalValueDelta += result4.UniqueValueBytesDelta

	expectedKeyDelta := int64(2 + 0 + 2 + 2 - 2 + 0)
	expectedValueDelta := int64(7 + 7 + 2 + 2 - 2 - 10)

	if totalKeyDelta != expectedKeyDelta {
		t.Errorf("total key delta mismatch: got %d, want %d", totalKeyDelta, expectedKeyDelta)
	}
	if totalValueDelta != expectedValueDelta {
		t.Errorf("total value delta mismatch: got %d, want %d", totalValueDelta, expectedValueDelta)
	}
}

func TestConcurrentOperations(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	const numGoroutines = 10
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	errs := make(chan error, numGoroutines*opsPerGoroutine)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				key := []byte(string(rune('a'+id)) + string(rune('0'+j%10)))
				value := []byte("value" + string(rune('0'+j)))

				_, err := db.Set(key, value)
				if err != nil {
					errs <- err
					continue
				}

				got, err := db.Get(key)
				if err != nil {
					errs <- err
					continue
				}
				if !bytes.Equal(got, value) {
					errs <- errors.New("concurrent read mismatch")
				}

				if j%3 == 0 {
					_, err = db.Delete(key)
					if err != nil {
						errs <- err
					}
				}
			}
		}(i)
	}

	wg.Wait()
	close(errs)

	var errCount int
	for err := range errs {
		t.Errorf("concurrent operation error: %v", err)
		errCount++
		if errCount > 10 {
			t.Fatal("too many concurrent errors")
		}
	}
}

func TestNilAndEmptyValues(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("empty key operations rejected by badger", func(t *testing.T) {
		_, err := db.Set(nil, []byte("value"))
		if err == nil {
			t.Error("expected error for nil key, badger rejects empty keys")
		}

		_, err = db.Set([]byte(""), []byte("value"))
		if err == nil {
			t.Error("expected error for empty key, badger rejects empty keys")
		}

		_, err = db.Get(nil)
		if err == nil {
			t.Error("expected error for nil key get")
		}

		_, err = db.Exists(nil)
		if err == nil {
			t.Error("expected error for nil key exists")
		}

		_, err = db.Delete(nil)
		if err == nil {
			t.Error("expected error for nil key delete")
		}
	})

	t.Run("nil value operations", func(t *testing.T) {
		key := []byte("nilvalue")
		result, err := db.Set(key, nil)
		if err != nil {
			t.Errorf("Set with nil value failed: %v", err)
		}
		if result.UniqueValueBytesDelta != 0 {
			t.Errorf("nil value should have 0 byte delta, got %d", result.UniqueValueBytesDelta)
		}

		got, err := db.Get(key)
		if err != nil {
			t.Errorf("Get key with nil value failed: %v", err)
		}
		if got != nil && len(got) != 0 {
			t.Errorf("expected nil or empty value, got %q", got)
		}
	})
}

func TestBatchOperationsEdgeCases(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("SetBatch with nil items", func(t *testing.T) {
		result, err := db.SetBatch(nil)
		if err != nil {
			t.Errorf("SetBatch with nil failed: %v", err)
		}
		if result.KeysUpdated != 0 || result.UniqueKeyBytesDelta != 0 || result.UniqueValueBytesDelta != 0 {
			t.Error("nil batch should have zero results")
		}
	})

	t.Run("DeleteBatch with nil items", func(t *testing.T) {
		result, err := db.DeleteBatch(nil)
		if err != nil {
			t.Errorf("DeleteBatch with nil failed: %v", err)
		}
		if result.KeysUpdated != 0 || result.UniqueKeyBytesDelta != 0 || result.UniqueValueBytesDelta != 0 {
			t.Error("nil batch should have zero results")
		}
	})

	t.Run("SetBatch with partial failures simulation", func(t *testing.T) {
		largeValue := make([]byte, 1<<20)
		items := []planar.KVItem{
			{Key: []byte("normal1"), Value: []byte("value1")},
			{Key: []byte("large"), Value: largeValue},
			{Key: []byte("normal2"), Value: []byte("value2")},
		}

		result, err := db.SetBatch(items)
		if err != nil {
			t.Logf("SetBatch error (expected for large values): %v", err)
		} else {
			if result.KeysUpdated != 3 {
				t.Errorf("expected 3 keys updated, got %d", result.KeysUpdated)
			}
		}
	})
}

func TestIterationEdgeCases(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()

	t.Run("IterateKeys with large offset", func(t *testing.T) {
		_, _ = db.Set([]byte("key1"), []byte("value1"))
		_, _ = db.Set([]byte("key2"), []byte("value2"))

		var count int
		err := db.IterateKeys(nil, 1000, 0, func(key []byte) (bool, error) {
			count++
			return true, nil
		})
		if err != nil {
			t.Errorf("IterateKeys with large offset failed: %v", err)
		}
		if count != 0 {
			t.Errorf("expected 0 keys with offset > total keys, got %d", count)
		}
	})

	t.Run("IterateItems with nil receiver", func(t *testing.T) {
		_, _ = db.Set([]byte("test"), []byte("value"))

		defer func() {
			if r := recover(); r != nil {
				t.Logf("Expected panic with nil receiver: %v", r)
			}
		}()

		err := db.IterateItems(planar.KVIterOptions{}, nil)
		if err == nil {
			t.Error("expected error or panic with nil receiver")
		}
	})
}

func TestClose(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := newBadgerKV(dbPath)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	_, err = db.Set([]byte("key"), []byte("value"))
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	if backend, ok := db.(*badgerBackend); ok {
		err = backend.db.Close()
		if err != nil {
			t.Errorf("Close failed: %v", err)
		}

		_, err = db.Get([]byte("key"))
		if err == nil {
			t.Error("expected error after close")
		}
	}
}

func setupTestDB(t *testing.T) (planar.KV, func()) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	db, err := newBadgerKV(dbPath)
	if err != nil {
		t.Fatalf("failed to create test db: %v", err)
	}

	cleanup := func() {
		if backend, ok := db.(*badgerBackend); ok {
			backend.db.Close()
		}
	}

	return db, cleanup
}
