package events

/*

These events are meant to be static permanent-use event topics that are for the local application to arbitarily define
and use within their specific context, and can not be overridden by the application.

*/
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
	MaxHandlers             = 1024 // max handlers per topic
	TotalEventSystemWorkers = 256  // we can have a lot of these so we need to limit them
)

const (
	TopicFlag = iota
	TopicReserved0
	TopicReserved1
	TopicReserved2
	TopicReserved3
	TopicReserved4
	TopicReserved5
	TopicReserved6
	TopicUsageSentinel
)

var _openTopic = TopicUsageSentinel
var _availableTopicCounter int
var _availableTopicCounterMu sync.Mutex
var _handlerIDCounter int64

func init() {
	_openTopic = TopicUsageSentinel
	_availableTopicCounter = _openTopic
}

func registerLocalTopicSlot() int {
	if _availableTopicCounter >= MaxLocalEventTopics {
		panic("max local event topics reached")
	}
	_availableTopicCounterMu.Lock()
	defer _availableTopicCounterMu.Unlock()
	_availableTopicCounter++
	return _availableTopicCounter
}

type Event struct {
	Header [4]byte // Header keys the consumer of the topic on how to process the data
	Body   interface{}
}

type EventHandler interface {
	OnEvent(event Event)
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

type LocalEventSystem struct {
	logger *slog.Logger
	topics []topicData

	ctx         context.Context
	workerPool  *pool.Pool
	knownTopics map[string]int
}

type TopicPublisher interface {
	Publish(event Event) // locked to the topic
}

type topicPublisherImpl struct {
	les   *LocalEventSystem
	topic int
}

var _ TopicPublisher = &topicPublisherImpl{}

// We lock the publisher to the topic to avoid lookups and to ensure that once set,
// the topic publisher is not changed and we can ensure as much movement as possible.
func (tpi *topicPublisherImpl) Publish(event Event) {
	// We dont lock because we explicitly dont allow deletion and only grow monotonically in indexing
	// so there is no race condition. We use atomic back counter for bounds checking.
	back := atomic.LoadInt32(&tpi.les.topics[tpi.topic].back)
	for i := int32(0); i < back; i++ {
		h := tpi.les.topics[tpi.topic].handlers[i] // capture for closure
		tpi.les.workerPool.Submit(func(ctx context.Context) error {
			h.callback.OnEvent(event)
			return nil
		})
	}
}

func NewLocalEventSystem(logger *slog.Logger, ctx context.Context) *LocalEventSystem {
	logger = logger.With("component", "events")

	topics := make([]topicData, MaxLocalEventTopics)
	for i := range topics {
		topics[i] = topicData{
			handlers: make([]handler, MaxHandlers),
			back:     0,
		}
	}

	knownTopics := map[string]int{
		"system-flag":      TopicFlag,
		"system-reserved0": TopicReserved0,
		"system-reserved1": TopicReserved1,
		"system-reserved2": TopicReserved2,
		"system-reserved3": TopicReserved3,
		"system-reserved4": TopicReserved4,
		"system-reserved5": TopicReserved5,
		"system-reserved6": TopicReserved6,
	}

	if len(knownTopics) != TopicUsageSentinel {
		panic("known topics does not match expected number of topics")
	}

	workerPool := pool.NewBuilder().WithLogger(logger).WithWorkers(TotalEventSystemWorkers).Build(ctx)

	les := &LocalEventSystem{
		logger:      logger,
		workerPool:  workerPool,
		topics:      topics,
		knownTopics: knownTopics,
		ctx:         ctx,
	}

	return les
}

func (les *LocalEventSystem) RegisterTopic(name string) error {
	if _, ok := les.knownTopics[name]; ok {
		return ErrTopicAlreadyRegistered
	}
	les.knownTopics[name] = registerLocalTopicSlot()
	return nil
}

func (les *LocalEventSystem) GetTopicPublisher(name string) (TopicPublisher, error) {
	if _, ok := les.knownTopics[name]; !ok {
		return nil, ErrTopicNotRegistered
	}
	return &topicPublisherImpl{
		les:   les,
		topic: les.knownTopics[name],
	}, nil
}

func (les *LocalEventSystem) AddHandler(topicName string, callback EventHandler) (string, error) {
	topicID, ok := les.knownTopics[topicName]
	if !ok {
		return "", ErrTopicNotRegistered
	}

	handlerID := fmt.Sprintf("handler-%d", atomic.AddInt64(&_handlerIDCounter, 1))

	// Atomically reserve a slot by incrementing back
	slot := atomic.AddInt32(&les.topics[topicID].back, 1) - 1
	if slot >= MaxHandlers {
		// This should never happen in normal operation, but safety check
		atomic.AddInt32(&les.topics[topicID].back, -1) // rollback
		panic("max handlers per topic reached")
	}

	h := handler{
		handlerUUID: handlerID,
		topic:       topicID,
		callback:    callback,
	}

	les.topics[topicID].handlers[slot] = h

	return handlerID, nil
}
