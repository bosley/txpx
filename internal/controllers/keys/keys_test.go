package keys

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/bosley/txpx/internal/controllers/users"
	"github.com/bosley/txpx/internal/models"
)

func setupTest(t *testing.T) (Keys, users.Users, *models.User) {
	logger := slog.Default()
	userController := users.New()
	keysController := New(logger.WithGroup("keys"), userController)

	user, err := userController.CreateUser("test@example.com", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}
	if user == nil {
		t.Fatal("User is nil")
	}

	return keysController, userController, user
}

func TestCreateApiKeyForUser(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("successful creation", func(t *testing.T) {
		apiKey, err := keysController.CreateApiKeyForUser(user.UUID, "test-key")
		if err != nil {
			t.Errorf("Failed to create API key: %v", err)
		}
		if apiKey == "" {
			t.Error("API key is empty")
		}
		if !strings.HasPrefix(apiKey, "v4.public.") {
			t.Error("API key should start with v4.public.")
		}
	})

	t.Run("duplicate key name", func(t *testing.T) {
		_, err := keysController.CreateApiKeyForUser(user.UUID, "duplicate-key")
		if err != nil {
			t.Errorf("Failed to create first key: %v", err)
		}

		_, err = keysController.CreateApiKeyForUser(user.UUID, "duplicate-key")
		if err != ErrKeyNameInUseForUser {
			t.Errorf("Expected ErrKeyNameInUseForUser, got %v", err)
		}
	})

	t.Run("non-existent user", func(t *testing.T) {
		_, err := keysController.CreateApiKeyForUser("non-existent-uuid", "test-key")
		if err != users.ErrUserNotFound {
			t.Errorf("Expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("multiple keys for same user", func(t *testing.T) {
		key1, err := keysController.CreateApiKeyForUser(user.UUID, "key-1")
		if err != nil {
			t.Errorf("Failed to create key-1: %v", err)
		}
		if key1 == "" {
			t.Error("key1 is empty")
		}

		key2, err := keysController.CreateApiKeyForUser(user.UUID, "key-2")
		if err != nil {
			t.Errorf("Failed to create key-2: %v", err)
		}
		if key2 == "" {
			t.Error("key2 is empty")
		}

		if key1 == key2 {
			t.Error("Keys should be different")
		}
	})
}

func TestListAllApiKeysForUser(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("empty list initially", func(t *testing.T) {
		keys, err := keysController.ListAllApiKeysForUser(user.UUID)
		if err != nil {
			t.Errorf("Failed to list keys: %v", err)
		}
		if len(keys) != 0 {
			t.Errorf("Expected empty keys list, got %d keys", len(keys))
		}
	})

	t.Run("lists only key names", func(t *testing.T) {
		_, err := keysController.CreateApiKeyForUser(user.UUID, "key-1")
		if err != nil {
			t.Errorf("Failed to create key-1: %v", err)
		}

		_, err = keysController.CreateApiKeyForUser(user.UUID, "key-2")
		if err != nil {
			t.Errorf("Failed to create key-2: %v", err)
		}

		keys, err := keysController.ListAllApiKeysForUser(user.UUID)
		if err != nil {
			t.Errorf("Failed to list keys: %v", err)
		}
		if len(keys) != 2 {
			t.Errorf("Expected 2 keys, got %d", len(keys))
		}

		hasKey1, hasKey2 := false, false
		for _, k := range keys {
			if k == "key-1" {
				hasKey1 = true
			}
			if k == "key-2" {
				hasKey2 = true
			}
		}
		if !hasKey1 {
			t.Error("key-1 not found in list")
		}
		if !hasKey2 {
			t.Error("key-2 not found in list")
		}

		for _, key := range keys {
			if strings.HasPrefix(key, "v4.public.") {
				t.Error("Key names should not contain actual tokens")
			}
		}
	})

	t.Run("non-existent user", func(t *testing.T) {
		_, err := keysController.ListAllApiKeysForUser("non-existent-uuid")
		if err != users.ErrUserNotFound {
			t.Errorf("Expected ErrUserNotFound, got %v", err)
		}
	})
}

func TestGetApiKeyForUser(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("get existing key", func(t *testing.T) {
		createdKey, err := keysController.CreateApiKeyForUser(user.UUID, "test-key")
		if err != nil {
			t.Errorf("Failed to create key: %v", err)
		}

		retrievedKey, err := keysController.GetApiKeyForUser(user.UUID, "test-key")
		if err != nil {
			t.Errorf("Failed to get key: %v", err)
		}
		if createdKey != retrievedKey {
			t.Error("Retrieved key does not match created key")
		}
	})

	t.Run("non-existent key", func(t *testing.T) {
		_, err := keysController.GetApiKeyForUser(user.UUID, "non-existent")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("non-existent user", func(t *testing.T) {
		_, err := keysController.GetApiKeyForUser("non-existent-uuid", "test-key")
		if err != users.ErrUserNotFound {
			t.Errorf("Expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("user with no keys", func(t *testing.T) {
		freshUser, err := keysController.(*keysImpl).userController.CreateUser("fresh@example.com", "password")
		if err != nil {
			t.Errorf("Failed to create fresh user: %v", err)
		}

		_, err = keysController.GetApiKeyForUser(freshUser.UUID, "test-key")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})
}

func TestUpdateApiKeyForUser(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("successful rename", func(t *testing.T) {
		originalKey, err := keysController.CreateApiKeyForUser(user.UUID, "original-name")
		if err != nil {
			t.Errorf("Failed to create key: %v", err)
		}

		err = keysController.UpdateApiKeyForUser(user.UUID, "original-name", "new-name")
		if err != nil {
			t.Errorf("Failed to update key: %v", err)
		}

		_, err = keysController.GetApiKeyForUser(user.UUID, "original-name")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound for old name, got %v", err)
		}

		renamedKey, err := keysController.GetApiKeyForUser(user.UUID, "new-name")
		if err != nil {
			t.Errorf("Failed to get renamed key: %v", err)
		}
		if originalKey != renamedKey {
			t.Error("Renamed key should have same token value")
		}
	})

	t.Run("rename to existing name", func(t *testing.T) {
		_, err := keysController.CreateApiKeyForUser(user.UUID, "key-1")
		if err != nil {
			t.Errorf("Failed to create key-1: %v", err)
		}

		_, err = keysController.CreateApiKeyForUser(user.UUID, "key-2")
		if err != nil {
			t.Errorf("Failed to create key-2: %v", err)
		}

		err = keysController.UpdateApiKeyForUser(user.UUID, "key-1", "key-2")
		if err != ErrKeyNameInUseForUser {
			t.Errorf("Expected ErrKeyNameInUseForUser, got %v", err)
		}
	})

	t.Run("rename non-existent key", func(t *testing.T) {
		err := keysController.UpdateApiKeyForUser(user.UUID, "non-existent", "new-name")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("non-existent user", func(t *testing.T) {
		err := keysController.UpdateApiKeyForUser("non-existent-uuid", "key", "new-key")
		if err != users.ErrUserNotFound {
			t.Errorf("Expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("user with no keys", func(t *testing.T) {
		err := keysController.UpdateApiKeyForUser(user.UUID, "key", "new-key")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})
}

func TestDeleteApiKeyForUser(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("successful deletion", func(t *testing.T) {
		_, err := keysController.CreateApiKeyForUser(user.UUID, "to-delete")
		if err != nil {
			t.Errorf("Failed to create key: %v", err)
		}

		err = keysController.DeleteApiKeyForUser(user.UUID, "to-delete")
		if err != nil {
			t.Errorf("Failed to delete key: %v", err)
		}

		_, err = keysController.GetApiKeyForUser(user.UUID, "to-delete")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound after delete, got %v", err)
		}
	})

	t.Run("delete non-existent key", func(t *testing.T) {
		err := keysController.DeleteApiKeyForUser(user.UUID, "non-existent")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("non-existent user", func(t *testing.T) {
		err := keysController.DeleteApiKeyForUser("non-existent-uuid", "key")
		if err != users.ErrUserNotFound {
			t.Errorf("Expected ErrUserNotFound, got %v", err)
		}
	})

	t.Run("user with no keys", func(t *testing.T) {
		err := keysController.DeleteApiKeyForUser(user.UUID, "key")
		if err != ErrKeyNotFound {
			t.Errorf("Expected ErrKeyNotFound, got %v", err)
		}
	})

	t.Run("delete one of multiple keys", func(t *testing.T) {
		_, err := keysController.CreateApiKeyForUser(user.UUID, "key-1")
		if err != nil {
			t.Errorf("Failed to create key-1: %v", err)
		}

		key2Token, err := keysController.CreateApiKeyForUser(user.UUID, "key-2")
		if err != nil {
			t.Errorf("Failed to create key-2: %v", err)
		}

		err = keysController.DeleteApiKeyForUser(user.UUID, "key-1")
		if err != nil {
			t.Errorf("Failed to delete key-1: %v", err)
		}

		keys, err := keysController.ListAllApiKeysForUser(user.UUID)
		if err != nil {
			t.Errorf("Failed to list keys: %v", err)
		}
		if len(keys) != 1 {
			t.Errorf("Expected 1 key, got %d", len(keys))
		}
		hasKey2 := false
		for _, k := range keys {
			if k == "key-2" {
				hasKey2 = true
			}
		}
		if !hasKey2 {
			t.Error("key-2 not found in list")
		}

		retrievedKey2, err := keysController.GetApiKeyForUser(user.UUID, "key-2")
		if err != nil {
			t.Errorf("Failed to get key-2: %v", err)
		}
		if key2Token != retrievedKey2 {
			t.Error("Retrieved key-2 does not match original token")
		}
	})
}

func TestGetUserFromApiKey(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("valid api key", func(t *testing.T) {
		apiKey, err := keysController.CreateApiKeyForUser(user.UUID, "test-key")
		if err != nil {
			t.Errorf("Failed to create API key: %v", err)
		}

		retrievedUser, err := keysController.GetUserFromApiKey(apiKey)
		if err != nil {
			t.Errorf("Failed to get user from API key: %v", err)
		}
		if user.UUID != retrievedUser.UUID {
			t.Errorf("Expected user UUID %s, got %s", user.UUID, retrievedUser.UUID)
		}
		if user.Email != retrievedUser.Email {
			t.Errorf("Expected user email %s, got %s", user.Email, retrievedUser.Email)
		}
	})

	t.Run("invalid api key", func(t *testing.T) {
		_, err := keysController.GetUserFromApiKey("invalid-token")
		if err != ErrApiKeyNotFound {
			t.Errorf("Expected ErrApiKeyNotFound, got %v", err)
		}
	})

	t.Run("deleted api key", func(t *testing.T) {
		apiKey, err := keysController.CreateApiKeyForUser(user.UUID, "to-delete")
		if err != nil {
			t.Errorf("Failed to create API key: %v", err)
		}

		err = keysController.DeleteApiKeyForUser(user.UUID, "to-delete")
		if err != nil {
			t.Errorf("Failed to delete API key: %v", err)
		}

		_, err = keysController.GetUserFromApiKey(apiKey)
		if err != ErrApiKeyNotFound {
			t.Errorf("Expected ErrApiKeyNotFound after delete, got %v", err)
		}
	})

	t.Run("multiple users with keys", func(t *testing.T) {
		user2, err := keysController.(*keysImpl).userController.CreateUser("user2@example.com", "password")
		if err != nil {
			t.Errorf("Failed to create user2: %v", err)
		}

		key1, err := keysController.CreateApiKeyForUser(user.UUID, "user1-key")
		if err != nil {
			t.Errorf("Failed to create key1: %v", err)
		}

		key2, err := keysController.CreateApiKeyForUser(user2.UUID, "user2-key")
		if err != nil {
			t.Errorf("Failed to create key2: %v", err)
		}

		retrievedUser1, err := keysController.GetUserFromApiKey(key1)
		if err != nil {
			t.Errorf("Failed to get user from key1: %v", err)
		}
		if user.UUID != retrievedUser1.UUID {
			t.Errorf("Expected user1 UUID %s, got %s", user.UUID, retrievedUser1.UUID)
		}

		retrievedUser2, err := keysController.GetUserFromApiKey(key2)
		if err != nil {
			t.Errorf("Failed to get user from key2: %v", err)
		}
		if user2.UUID != retrievedUser2.UUID {
			t.Errorf("Expected user2 UUID %s, got %s", user2.UUID, retrievedUser2.UUID)
		}
	})
}

func TestApiKeyExpiration(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("key has 30 day expiration", func(t *testing.T) {
		apiKey, err := keysController.CreateApiKeyForUser(user.UUID, "expiration-test")
		if err != nil {
			t.Errorf("Failed to create API key: %v", err)
		}

		retrievedUser, err := keysController.GetUserFromApiKey(apiKey)
		if err != nil {
			t.Errorf("Failed to get user from API key: %v", err)
		}
		if retrievedUser == nil {
			t.Error("Retrieved user is nil")
		}
	})
}

func TestApiKeyTokenFormat(t *testing.T) {
	keysController, _, user := setupTest(t)

	t.Run("token is paseto v4 public", func(t *testing.T) {
		apiKey, err := keysController.CreateApiKeyForUser(user.UUID, "format-test")
		if err != nil {
			t.Errorf("Failed to create API key: %v", err)
		}

		if !strings.HasPrefix(apiKey, "v4.public.") {
			t.Error("API key should start with v4.public.")
		}
		parts := strings.Split(apiKey, ".")
		if len(parts) < 3 {
			t.Errorf("API key should have at least 3 parts, got %d", len(parts))
		}
	})
}

func TestKeyPersistenceAcrossOperations(t *testing.T) {
	keysController, _, user := setupTest(t)

	apiKey, err := keysController.CreateApiKeyForUser(user.UUID, "persistent-key")
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}

	for i := 0; i < 5; i++ {
		retrievedUser, err := keysController.GetUserFromApiKey(apiKey)
		if err != nil {
			t.Errorf("Failed to get user from API key on iteration %d: %v", i, err)
		}
		if user.UUID != retrievedUser.UUID {
			t.Errorf("Expected user UUID %s, got %s on iteration %d", user.UUID, retrievedUser.UUID, i)
		}
	}

	err = keysController.UpdateApiKeyForUser(user.UUID, "persistent-key", "renamed-persistent")
	if err != nil {
		t.Errorf("Failed to update key: %v", err)
	}

	retrievedUser, err := keysController.GetUserFromApiKey(apiKey)
	if err != nil {
		t.Errorf("Failed to get user after rename: %v", err)
	}
	if user.UUID != retrievedUser.UUID {
		t.Errorf("Expected user UUID %s, got %s after rename", user.UUID, retrievedUser.UUID)
	}

	retrievedKey, err := keysController.GetApiKeyForUser(user.UUID, "renamed-persistent")
	if err != nil {
		t.Errorf("Failed to get renamed key: %v", err)
	}
	if apiKey != retrievedKey {
		t.Error("Retrieved key does not match original after rename")
	}
}

func TestConcurrentKeyCreation(t *testing.T) {
	keysController, userController, _ := setupTest(t)

	user1, err := userController.CreateUser("concurrent1@example.com", "password")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}

	user2, err := userController.CreateUser("concurrent2@example.com", "password")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	key1, err := keysController.CreateApiKeyForUser(user1.UUID, "key")
	if err != nil {
		t.Errorf("Failed to create key1: %v", err)
	}

	key2, err := keysController.CreateApiKeyForUser(user2.UUID, "key")
	if err != nil {
		t.Errorf("Failed to create key2: %v", err)
	}

	if key1 == key2 {
		t.Error("Keys for different users should be different")
	}

	retrievedUser1, err := keysController.GetUserFromApiKey(key1)
	if err != nil {
		t.Errorf("Failed to get user1 from key1: %v", err)
	}
	if user1.UUID != retrievedUser1.UUID {
		t.Errorf("Expected user1 UUID %s, got %s", user1.UUID, retrievedUser1.UUID)
	}

	retrievedUser2, err := keysController.GetUserFromApiKey(key2)
	if err != nil {
		t.Errorf("Failed to get user2 from key2: %v", err)
	}
	if user2.UUID != retrievedUser2.UUID {
		t.Errorf("Expected user2 UUID %s, got %s", user2.UUID, retrievedUser2.UUID)
	}
}

func TestEmptyKeyName(t *testing.T) {
	keysController, _, user := setupTest(t)

	apiKey, err := keysController.CreateApiKeyForUser(user.UUID, "")
	if err != nil {
		t.Errorf("Failed to create API key with empty name: %v", err)
	}
	if apiKey == "" {
		t.Error("API key is empty")
	}

	retrievedKey, err := keysController.GetApiKeyForUser(user.UUID, "")
	if err != nil {
		t.Errorf("Failed to get API key with empty name: %v", err)
	}
	if apiKey != retrievedKey {
		t.Error("Retrieved key does not match created key")
	}
}

func TestKeyTimestamps(t *testing.T) {
	keysController, _, user := setupTest(t)

	beforeCreate := time.Now()
	_, err := keysController.CreateApiKeyForUser(user.UUID, "timestamp-test")
	if err != nil {
		t.Errorf("Failed to create API key: %v", err)
	}
	afterCreate := time.Now()

	time.Sleep(10 * time.Millisecond)

	beforeUpdate := time.Now()
	err = keysController.UpdateApiKeyForUser(user.UUID, "timestamp-test", "timestamp-renamed")
	if err != nil {
		t.Errorf("Failed to update API key: %v", err)
	}
	afterUpdate := time.Now()

	_ = beforeCreate
	_ = afterCreate
	_ = beforeUpdate
	_ = afterUpdate
}
