package websession

import (
	"log/slog"
	"testing"
	"time"

	"github.com/bosley/txpx/internal/controllers/users"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTest(t *testing.T) WebSession {
	logger := slog.Default()
	userController := users.New()
	return New(logger, userController)
}

func TestNewWebSession(t *testing.T) {
	ws := setupTest(t)
	userUUID := "test-user-123"
	expiresAt := time.Now().Add(24 * time.Hour)

	session, err := ws.NewWebSession(userUUID, expiresAt)
	require.NoError(t, err)
	assert.NotEmpty(t, session.UUID)
	assert.Equal(t, userUUID, session.UserUUID)
	assert.Equal(t, expiresAt.Unix(), session.ExpiresAt.Unix())
}

func TestListAllUsersWebSessions(t *testing.T) {
	ws := setupTest(t)
	userUUID1 := "user-1"
	userUUID2 := "user-2"

	_, err := ws.ListAllUsersWebSessions(userUUID1)
	assert.ErrorIs(t, err, ErrWebSessionNotFound)

	session1, err := ws.NewWebSession(userUUID1, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	session2, err := ws.NewWebSession(userUUID1, time.Now().Add(2*time.Hour))
	require.NoError(t, err)
	_, err = ws.NewWebSession(userUUID2, time.Now().Add(3*time.Hour))
	require.NoError(t, err)

	sessions, err := ws.ListAllUsersWebSessions(userUUID1)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)
	assert.Contains(t, sessions, session1)
	assert.Contains(t, sessions, session2)

	sessions, err = ws.ListAllUsersWebSessions(userUUID2)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
}

func TestGetWebSession(t *testing.T) {
	ws := setupTest(t)
	userUUID := "test-user"
	expiresAt := time.Now().Add(1 * time.Hour)

	_, err := ws.GetWebSession("non-existent")
	assert.ErrorIs(t, err, ErrWebSessionNotFound)

	session, err := ws.NewWebSession(userUUID, expiresAt)
	require.NoError(t, err)

	retrieved, err := ws.GetWebSession(session.UUID)
	require.NoError(t, err)
	assert.Equal(t, session.UUID, retrieved.UUID)
	assert.Equal(t, session.UserUUID, retrieved.UserUUID)
	assert.Equal(t, session.ExpiresAt.Unix(), retrieved.ExpiresAt.Unix())
}

func TestDeleteWebSession(t *testing.T) {
	ws := setupTest(t)
	userUUID := "test-user"

	session1, err := ws.NewWebSession(userUUID, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	session2, err := ws.NewWebSession(userUUID, time.Now().Add(2*time.Hour))
	require.NoError(t, err)

	sessions, err := ws.ListAllUsersWebSessions(userUUID)
	require.NoError(t, err)
	assert.Len(t, sessions, 2)

	err = ws.DeleteWebSession(session1.UUID)
	require.NoError(t, err)

	sessions, err = ws.ListAllUsersWebSessions(userUUID)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
	assert.Equal(t, session2.UUID, sessions[0].UUID)

	_, err = ws.GetWebSession(session1.UUID)
	assert.ErrorIs(t, err, ErrWebSessionNotFound)

	err = ws.DeleteWebSession("non-existent")
	assert.NoError(t, err)

	err = ws.DeleteWebSession(session1.UUID)
	assert.NoError(t, err)
}

func TestHasWebSession(t *testing.T) {
	ws := setupTest(t)
	userUUID := "test-user"

	has, err := ws.HasWebSession("non-existent")
	require.NoError(t, err)
	assert.False(t, has)

	session, err := ws.NewWebSession(userUUID, time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	has, err = ws.HasWebSession(session.UUID)
	require.NoError(t, err)
	assert.True(t, has)

	err = ws.DeleteWebSession(session.UUID)
	require.NoError(t, err)

	has, err = ws.HasWebSession(session.UUID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestPruneAllExpiredWebSessions(t *testing.T) {
	ws := setupTest(t)
	userUUID1 := "user-1"
	userUUID2 := "user-2"

	expired1, err := ws.NewWebSession(userUUID1, time.Now().Add(-2*time.Hour))
	require.NoError(t, err)
	active1, err := ws.NewWebSession(userUUID1, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	expired2, err := ws.NewWebSession(userUUID2, time.Now().Add(-1*time.Hour))
	require.NoError(t, err)
	active2, err := ws.NewWebSession(userUUID2, time.Now().Add(2*time.Hour))
	require.NoError(t, err)

	err = ws.PruneAllExpiredWebSessions()
	require.NoError(t, err)

	sessions1, err := ws.ListAllUsersWebSessions(userUUID1)
	require.NoError(t, err)
	assert.Len(t, sessions1, 1)
	assert.Equal(t, active1.UUID, sessions1[0].UUID)

	sessions2, err := ws.ListAllUsersWebSessions(userUUID2)
	require.NoError(t, err)
	assert.Len(t, sessions2, 1)
	assert.Equal(t, active2.UUID, sessions2[0].UUID)

	has, err := ws.HasWebSession(expired1.UUID)
	require.NoError(t, err)
	assert.False(t, has)

	has, err = ws.HasWebSession(expired2.UUID)
	require.NoError(t, err)
	assert.False(t, has)
}

func TestMultipleSessionsPerUser(t *testing.T) {
	ws := setupTest(t)
	userUUID := "multi-session-user"

	session1, err := ws.NewWebSession(userUUID, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	session2, err := ws.NewWebSession(userUUID, time.Now().Add(2*time.Hour))
	require.NoError(t, err)
	session3, err := ws.NewWebSession(userUUID, time.Now().Add(3*time.Hour))
	require.NoError(t, err)

	sessions, err := ws.ListAllUsersWebSessions(userUUID)
	require.NoError(t, err)
	assert.Len(t, sessions, 3)

	for _, s := range []*struct{ UUID string }{
		{session1.UUID}, {session2.UUID}, {session3.UUID},
	} {
		has, err := ws.HasWebSession(s.UUID)
		require.NoError(t, err)
		assert.True(t, has)
	}
}

func TestIsolationBetweenUsers(t *testing.T) {
	ws := setupTest(t)
	user1 := "user-isolation-1"
	user2 := "user-isolation-2"

	session1, err := ws.NewWebSession(user1, time.Now().Add(1*time.Hour))
	require.NoError(t, err)
	session2, err := ws.NewWebSession(user2, time.Now().Add(1*time.Hour))
	require.NoError(t, err)

	sessions1, err := ws.ListAllUsersWebSessions(user1)
	require.NoError(t, err)
	assert.Len(t, sessions1, 1)
	assert.Equal(t, session1.UUID, sessions1[0].UUID)

	sessions2, err := ws.ListAllUsersWebSessions(user2)
	require.NoError(t, err)
	assert.Len(t, sessions2, 1)
	assert.Equal(t, session2.UUID, sessions2[0].UUID)

	err = ws.DeleteWebSession(session1.UUID)
	require.NoError(t, err)

	sessions2After, err := ws.ListAllUsersWebSessions(user2)
	require.NoError(t, err)
	assert.Len(t, sessions2After, 1)
	assert.Equal(t, session2.UUID, sessions2After[0].UUID)
}
