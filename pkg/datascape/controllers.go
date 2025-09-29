package datascape

import (
	"log/slog"

	"github.com/bosley/txpx/pkg/datascape/keys"
	"github.com/bosley/txpx/pkg/datascape/users"
	"github.com/bosley/txpx/pkg/datascape/websession"
)

type Controllers interface {
	GetUserController() users.Users
	GetKeysController() keys.Keys
	GetWebsessionController() websession.WebSession
}

type controllersImpl struct {
	logger *slog.Logger

	userController       users.Users
	keysController       keys.Keys
	websessionController websession.WebSession
}

var _ Controllers = &controllersImpl{}

func New(logger *slog.Logger) Controllers {

	userController := users.New(logger.WithGroup("users"))
	keysController := keys.New(logger.WithGroup("keys"), userController)
	websessionController := websession.New(logger.WithGroup("websession"), userController)

	return &controllersImpl{
		logger:               logger,
		userController:       userController,
		keysController:       keysController,
		websessionController: websessionController,
	}
}

func (x *controllersImpl) GetUserController() users.Users {
	return x.userController
}

func (x *controllersImpl) GetKeysController() keys.Keys {
	return x.keysController
}

func (x *controllersImpl) GetWebsessionController() websession.WebSession {
	return x.websessionController
}
