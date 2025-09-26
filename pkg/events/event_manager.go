package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/bosley/txpx/pkg/pool"
)

var (
	ErrTopicNotRegistered     = errors.New("topic not registered")
	ErrTopicAlreadyRegistered = errors.New("topic already registered")
)

const (
	MaxLocalEventTopics     = 2048
	TotalEventSystemWorkers = 256 // we can have a lot of these so we need to limit them
)

type Event struct {
	Header [4]byte // Header keys the consumer of the topic on how to process the data
	Body   interface{}
}

type EventHandler interface {
	OnEvent(event Event)
}

type TopicPublisher interface {
	Publish(event Event) // locked to the topic
}

type handler struct {
	handlerUUID string
	topic       int
	callback    EventHandler
}

type topicData struct {
	handlers []handler
	back     int32 // atomic access - current count of valid handlers
}

type EventManagerConfig struct {
	MaxTopics           int
	MaxHandlersPerTopic int
	WorkerPoolSize      int
	Logger              *slog.Logger
}

func DefaultEventManagerConfig(logger *slog.Logger) *EventManagerConfig {
	return &EventManagerConfig{
		MaxTopics:           2048,
		MaxHandlersPerTopic: 1024,
		WorkerPoolSize:      256,
		Logger:              logger.WithGroup("events"),
	}
}

type EventManager struct {
	config       *EventManagerConfig
	logger       *slog.Logger
	topics       []topicData
	topicsMu     sync.RWMutex
	knownTopics  map[string]int
	topicCounter int32

	ctx          context.Context
	workerPool   *pool.Pool
	handlerIDGen atomic.Int64

	systemTopics map[string]int
}

func NewEventManager(ctx context.Context, config *EventManagerConfig) *EventManager {
	if config == nil {
		panic("EventManagerConfig cannot be nil")
	}

	topics := make([]topicData, config.MaxTopics)
	for i := range topics {
		topics[i] = topicData{
			handlers: make([]handler, config.MaxHandlersPerTopic),
			back:     0,
		}
	}

	workerPool := pool.NewBuilder().
		WithLogger(config.Logger).
		WithWorkers(config.WorkerPoolSize).
		Build(ctx)

	em := &EventManager{
		config:       config,
		logger:       config.Logger,
		topics:       topics,
		knownTopics:  make(map[string]int),
		topicCounter: int32(0),
		ctx:          ctx,
		workerPool:   workerPool,
		systemTopics: make(map[string]int),
	}

	return em
}

func (em *EventManager) RegisterTopic(name string) error {
	em.topicsMu.Lock()
	defer em.topicsMu.Unlock()

	if _, exists := em.knownTopics[name]; exists {
		return ErrTopicAlreadyRegistered
	}

	if em.topicCounter >= int32(em.config.MaxTopics) {
		return errors.New("maximum topics reached")
	}

	em.knownTopics[name] = int(em.topicCounter)
	em.topicCounter++
	return nil
}

func (em *EventManager) GetTopicPublisher(name string) (TopicPublisher, error) {
	em.topicsMu.RLock()
	topicID, exists := em.knownTopics[name]
	em.topicsMu.RUnlock()

	if !exists {
		return nil, ErrTopicNotRegistered
	}

	return &managedTopicPublisher{
		em:    em,
		topic: topicID,
	}, nil
}

func (em *EventManager) AddHandler(topicName string, callback EventHandler) (string, error) {
	em.topicsMu.RLock()
	topicID, exists := em.knownTopics[topicName]
	em.topicsMu.RUnlock()

	if !exists {
		return "", ErrTopicNotRegistered
	}

	handlerID := fmt.Sprintf("handler-%d", em.handlerIDGen.Add(1))

	slot := atomic.AddInt32(&em.topics[topicID].back, 1) - 1
	if slot >= int32(em.config.MaxHandlersPerTopic) {
		atomic.AddInt32(&em.topics[topicID].back, -1)
		return "", errors.New("max handlers per topic reached")
	}

	h := handler{
		handlerUUID: handlerID,
		topic:       topicID,
		callback:    callback,
	}

	em.topics[topicID].handlers[slot] = h

	return handlerID, nil
}

func (em *EventManager) PublishToTopic(topicName string, event Event) error {
	em.topicsMu.RLock()
	topicID, exists := em.knownTopics[topicName]
	em.topicsMu.RUnlock()

	if !exists {
		return ErrTopicNotRegistered
	}

	em.publishToTopicID(topicID, event)
	return nil
}

func (em *EventManager) publishToTopicID(topicID int, event Event) {
	back := atomic.LoadInt32(&em.topics[topicID].back)
	for i := int32(0); i < back; i++ {
		h := em.topics[topicID].handlers[i]
		em.workerPool.Submit(func(ctx context.Context) error {
			h.callback.OnEvent(event)
			return nil
		})
	}
}

func (em *EventManager) TopicCount() int {
	em.topicsMu.RLock()
	defer em.topicsMu.RUnlock()
	return len(em.knownTopics)
}

func (em *EventManager) HandlerCount(topicName string) int {
	em.topicsMu.RLock()
	topicID, exists := em.knownTopics[topicName]
	em.topicsMu.RUnlock()

	if !exists {
		return 0
	}

	return int(atomic.LoadInt32(&em.topics[topicID].back))
}

type managedTopicPublisher struct {
	em    *EventManager
	topic int
}

func (mtp *managedTopicPublisher) Publish(event Event) {
	mtp.em.publishToTopicID(mtp.topic, event)
}

var _ TopicPublisher = &managedTopicPublisher{}
