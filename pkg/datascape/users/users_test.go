package users

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) Users {
	logger := slog.Default()
	return New(logger)
}

func TestCreateUser(t *testing.T) {
	u := setupTest(t)

	user, err := u.CreateUser("test@example.com", "password123")
	require.NoError(t, err)
	assert.NotEmpty(t, user.UUID)
	assert.Equal(t, "test@example.com", user.Email)
	assert.NotEqual(t, "password123", user.Password)
	assert.NotEmpty(t, user.Password)
	assert.False(t, user.CreatedAt.IsZero())
	assert.False(t, user.UpdatedAt.IsZero())

	_, err = u.CreateUser("test@example.com", "different")
	assert.ErrorIs(t, err, ErrEmailInUse)
}

func TestGetUserByEmail(t *testing.T) {
	u := setupTest(t)

	_, err := u.GetUserByEmail("nonexistent@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)

	created, err := u.CreateUser("find@example.com", "password")
	require.NoError(t, err)

	found, err := u.GetUserByEmail("find@example.com")
	require.NoError(t, err)
	assert.Equal(t, created.UUID, found.UUID)
	assert.Equal(t, created.Email, found.Email)
	assert.Equal(t, created.Password, found.Password)
}

func TestGetUserByUUID(t *testing.T) {
	u := setupTest(t)

	_, err := u.GetUserByUUID("nonexistent-uuid")
	assert.ErrorIs(t, err, ErrUserNotFound)

	created, err := u.CreateUser("uuid@example.com", "password")
	require.NoError(t, err)

	found, err := u.GetUserByUUID(created.UUID)
	require.NoError(t, err)
	assert.Equal(t, created.UUID, found.UUID)
	assert.Equal(t, created.Email, found.Email)
}

func TestListAllUsers(t *testing.T) {
	u := setupTest(t)

	users, err := u.ListAllUsers()
	require.NoError(t, err)
	assert.Len(t, users, 0)

	user1, err := u.CreateUser("user1@example.com", "pass1")
	require.NoError(t, err)
	user2, err := u.CreateUser("user2@example.com", "pass2")
	require.NoError(t, err)
	user3, err := u.CreateUser("user3@example.com", "pass3")
	require.NoError(t, err)

	users, err = u.ListAllUsers()
	require.NoError(t, err)
	assert.Len(t, users, 3)

	uuids := make(map[string]bool)
	for _, user := range users {
		uuids[user.UUID] = true
	}
	assert.True(t, uuids[user1.UUID])
	assert.True(t, uuids[user2.UUID])
	assert.True(t, uuids[user3.UUID])
}

func TestUpdateUserName(t *testing.T) {
	u := setupTest(t)

	_, err := u.UpdateUserName("nonexistent", "newname")
	assert.ErrorIs(t, err, ErrUserNotFound)

	user, err := u.CreateUser("name@example.com", "password")
	require.NoError(t, err)
	originalUpdatedAt := user.UpdatedAt

	time.Sleep(10 * time.Millisecond)

	updated, err := u.UpdateUserName(user.UUID, "John Doe")
	require.NoError(t, err)
	assert.Equal(t, "John Doe", updated.Name)
	assert.True(t, updated.UpdatedAt.After(originalUpdatedAt))

	fetched, err := u.GetUserByUUID(user.UUID)
	require.NoError(t, err)
	assert.Equal(t, "John Doe", fetched.Name)
}

func TestUpdateUserEmail(t *testing.T) {
	u := setupTest(t)

	_, err := u.UpdateUserEmail("nonexistent", "new@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)

	user1, err := u.CreateUser("email1@example.com", "password")
	require.NoError(t, err)
	_, err = u.CreateUser("email2@example.com", "password")
	require.NoError(t, err)

	_, err = u.UpdateUserEmail(user1.UUID, "email2@example.com")
	assert.ErrorIs(t, err, ErrEmailInUse)

	updated, err := u.UpdateUserEmail(user1.UUID, "newemail@example.com")
	require.NoError(t, err)
	assert.Equal(t, "newemail@example.com", updated.Email)

	_, err = u.GetUserByEmail("email1@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)

	fetched, err := u.GetUserByEmail("newemail@example.com")
	require.NoError(t, err)
	assert.Equal(t, user1.UUID, fetched.UUID)

	_, err = u.CreateUser("email1@example.com", "password")
	require.NoError(t, err)
}

func TestUpdateUserPassword(t *testing.T) {
	u := setupTest(t)

	_, err := u.UpdateUserPassword("nonexistent", "newpass")
	assert.ErrorIs(t, err, ErrUserNotFound)

	user, err := u.CreateUser("pass@example.com", "oldpassword")
	require.NoError(t, err)
	oldPasswordHash := user.Password

	updated, err := u.UpdateUserPassword(user.UUID, "newpassword")
	require.NoError(t, err)
	assert.NotEqual(t, oldPasswordHash, updated.Password)
	assert.NotEqual(t, "newpassword", updated.Password)

	fetched, err := u.GetUserByUUID(user.UUID)
	require.NoError(t, err)
	assert.Equal(t, updated.Password, fetched.Password)
}

func TestDeleteUser(t *testing.T) {
	u := setupTest(t)

	err := u.DeleteUser("nonexistent")
	assert.ErrorIs(t, err, ErrUserNotFound)

	user, err := u.CreateUser("delete@example.com", "password")
	require.NoError(t, err)

	err = u.DeleteUser(user.UUID)
	require.NoError(t, err)

	_, err = u.GetUserByUUID(user.UUID)
	assert.ErrorIs(t, err, ErrUserNotFound)

	_, err = u.GetUserByEmail("delete@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)

	newUser, err := u.CreateUser("delete@example.com", "password")
	require.NoError(t, err)
	assert.NotEqual(t, user.UUID, newUser.UUID)
}

func TestHashAndVerifyPassword(t *testing.T) {
	passwords := []string{
		"simple",
		"complex!@#$%^&*()",
		"with spaces in it",
		"1234567890123456789012345678901234567890123456789012345678901234567890ab",
	}

	for _, password := range passwords {
		hashed, err := HashUserPassword(password)
		require.NoError(t, err, "Failed to hash password: %q", password)
		assert.NotEmpty(t, hashed)
		assert.NotEqual(t, password, hashed)

		assert.True(t, VerifyUserPassword(password, hashed), "Failed to verify password: %q", password)
		assert.False(t, VerifyUserPassword("wrong", hashed))

		if len(password) < 72 {
			assert.False(t, VerifyUserPassword(password+"x", hashed))
		}
	}

	emptyHashed, err := HashUserPassword("")
	require.NoError(t, err)
	assert.NotEmpty(t, emptyHashed)
	assert.True(t, VerifyUserPassword("", emptyHashed))

	tooLong := "1234567890123456789012345678901234567890123456789012345678901234567890abc"
	_, err = HashUserPassword(tooLong)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "72 bytes")

	exactly72 := "1234567890123456789012345678901234567890123456789012345678901234567890ab"
	hash72, err := HashUserPassword(exactly72)
	require.NoError(t, err)
	assert.True(t, VerifyUserPassword(exactly72+"ignored", hash72), "bcrypt truncates at 72 bytes")

	hash1, err := HashUserPassword("same")
	require.NoError(t, err)
	hash2, err := HashUserPassword("same")
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash2)
}

func TestUserDataIntegrity(t *testing.T) {
	u := setupTest(t)

	user, err := u.CreateUser("integrity@example.com", "password")
	require.NoError(t, err)

	_, err = u.UpdateUserName(user.UUID, "Alice")
	require.NoError(t, err)

	_, err = u.UpdateUserEmail(user.UUID, "alice@example.com")
	require.NoError(t, err)

	_, err = u.UpdateUserPassword(user.UUID, "newpassword")
	require.NoError(t, err)

	final, err := u.GetUserByUUID(user.UUID)
	require.NoError(t, err)
	assert.Equal(t, "Alice", final.Name)
	assert.Equal(t, "alice@example.com", final.Email)
	assert.True(t, VerifyUserPassword("newpassword", final.Password))
	assert.False(t, VerifyUserPassword("password", final.Password))

	finalByEmail, err := u.GetUserByEmail("alice@example.com")
	require.NoError(t, err)
	assert.Equal(t, final.UUID, finalByEmail.UUID)
}

func TestMultipleUsersIsolation(t *testing.T) {
	u := setupTest(t)

	users := make(map[string]string)
	for i := 0; i < 5; i++ {
		email := string(rune('a'+i)) + "@example.com"
		user, err := u.CreateUser(email, "pass"+string(rune('0'+i)))
		require.NoError(t, err)
		users[email] = user.UUID
	}

	for email, uuid := range users {
		user, err := u.GetUserByEmail(email)
		require.NoError(t, err)
		assert.Equal(t, uuid, user.UUID)
		assert.Equal(t, email, user.Email)
	}

	err := u.DeleteUser(users["c@example.com"])
	require.NoError(t, err)

	allUsers, err := u.ListAllUsers()
	require.NoError(t, err)
	assert.Len(t, allUsers, 4)

	_, err = u.GetUserByEmail("c@example.com")
	assert.ErrorIs(t, err, ErrUserNotFound)
}
