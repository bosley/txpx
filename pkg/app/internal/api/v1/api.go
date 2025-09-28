package api

import (
	"errors"
	"log/slog"

	"github.com/bosley/txpx/pkg/events"
)

var (
	ErrEventPublisherNil = errors.New("event publisher is nil")
	ErrHeaderEmpty       = errors.New("header is empty")
)

/*

	This is the generic API for the generic app runtime application.
	Essentially, we are taking in commands from potential HTTP api,
	and potentially insi events.

	This API converts these api calls into application commands,
	and routes them to a supplied event publisher (locked to the topic)
	so that the implementation of the application can choose
	where they go and how to process them.
*/

const (
	EventTopic = ":internal:txpx-app-api"
)

type DataOrigin string

const (
	DataOriginHTTP DataOrigin = "http"
	DataOriginInsi DataOrigin = "insi"
)

type ApiCommandId string

const (
	// Only items emitted to the event system are considered commands
	ApiCommandIdRuntimeShutdown ApiCommandId = "runtime-shutdown"
)

type ApiRequest struct {
	Id   ApiCommandId
	Data interface{}
}

type Msg struct {
	UUID    string
	Origin  DataOrigin
	Request ApiRequest
}

type APISubmitter interface {
	Submit(msg Msg) error
}

type API interface {
	NewSubmiter(
		header [4]byte,
		origin DataOrigin,
	) APISubmitter
}

func New(
	logger *slog.Logger,
	publisher events.TopicPublisher,
) API {
	a := &apiImpl{
		logger:         logger,
		eventPublisher: publisher,
	}

	return a
}

type apiImpl struct {
	logger         *slog.Logger
	eventPublisher events.TopicPublisher
}

type apiSubmitterImpl struct {
	logger         *slog.Logger
	eventPublisher events.TopicPublisher
	header         [4]byte
}

func (a *apiSubmitterImpl) Submit(msg Msg) error {
	if a.eventPublisher == nil {
		return ErrEventPublisherNil
	}
	if a.header == [4]byte{} {
		return ErrHeaderEmpty
	}
	a.eventPublisher.Publish(events.Event{
		Header: a.header,
		Body:   msg,
	})
	return nil
}

func (a *apiImpl) NewSubmiter(
	header [4]byte,
	origin DataOrigin,

) APISubmitter {
	return &apiSubmitterImpl{
		logger:         a.logger,
		header:         header,
		eventPublisher: a.eventPublisher,
	}
}
