package websession

import (
	"errors"
	"log/slog"
	"time"

	"github.com/bosley/txpx/internal/controllers/users"
	"github.com/bosley/txpx/internal/models"
	"github.com/google/uuid"
)

var (
	ErrWebSessionNotFound = errors.New("web session not found")
)

type WebSession interface {
	NewWebSession(userUUID string, expiresAt time.Time) (*models.UserSession, error)
	ListAllUsersWebSessions(userUUID string) ([]*models.UserSession, error)
	GetWebSession(sessionUUID string) (*models.UserSession, error)
	DeleteWebSession(sessionUUID string) error
	HasWebSession(sessionUUID string) (bool, error)
	PruneAllExpiredWebSessions() error
}

type webSessionImpl struct {
	logger         *slog.Logger
	userController users.Users

	mockWebSessions map[string][]*models.UserSession
}

var _ WebSession = &webSessionImpl{}

func New(logger *slog.Logger, userController users.Users) WebSession {
	return &webSessionImpl{
		logger:         logger,
		userController: userController,

		// TEMP until site is ready for backend impl
		mockWebSessions: make(map[string][]*models.UserSession),
	}
}

// -- imple

func (x *webSessionImpl) NewWebSession(userUUID string, expiresAt time.Time) (*models.UserSession, error) {
	session := &models.UserSession{
		UUID:      uuid.New().String(),
		UserUUID:  userUUID,
		ExpiresAt: expiresAt,
	}
	if _, exists := x.mockWebSessions[userUUID]; !exists {
		x.mockWebSessions[userUUID] = make([]*models.UserSession, 0)
	}
	x.mockWebSessions[userUUID] = append(x.mockWebSessions[userUUID], session)
	return session, nil
}

func (x *webSessionImpl) ListAllUsersWebSessions(userUUID string) ([]*models.UserSession, error) {

	if _, exists := x.mockWebSessions[userUUID]; !exists {
		return nil, ErrWebSessionNotFound
	}
	return x.mockWebSessions[userUUID], nil
}

func (x *webSessionImpl) GetWebSession(sessionUUID string) (*models.UserSession, error) {
	for _, session := range x.mockWebSessions {
		for _, s := range session {
			if s.UUID == sessionUUID {
				return s, nil
			}
		}
	}
	return nil, ErrWebSessionNotFound
}

func (x *webSessionImpl) DeleteWebSession(sessionUUID string) error {
	newSessions := make(map[string][]*models.UserSession)
	for userUUID, session := range x.mockWebSessions {
		newSessions[userUUID] = make([]*models.UserSession, 0)
		for _, s := range session {
			if s.UUID == sessionUUID {
				continue
			}
			newSessions[userUUID] = append(newSessions[userUUID], s)
		}
	}
	x.mockWebSessions = newSessions
	return nil // idempotent
}

func (x *webSessionImpl) HasWebSession(sessionUUID string) (bool, error) {
	for _, session := range x.mockWebSessions {
		for _, s := range session {
			if s.UUID == sessionUUID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (x *webSessionImpl) PruneAllExpiredWebSessions() error {

	newSessions := make(map[string][]*models.UserSession)
	for userUUID, session := range x.mockWebSessions {
		newSessions[userUUID] = make([]*models.UserSession, 0)
		for _, s := range session {
			if s.ExpiresAt.After(time.Now()) {
				newSessions[userUUID] = append(newSessions[userUUID], s)
			}
		}
	}
	x.mockWebSessions = newSessions
	return nil
}
