package events

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testHandler struct {
	received int64
	id       string
}

func (th *testHandler) OnEvent(event Event) {
	atomic.AddInt64(&th.received, 1)
}

func TestEventManagerHighThroughput(t *testing.T) {
	ctx := context.Background()
	config := DefaultEventManagerConfig(slog.Default())
	em := NewEventManager(ctx, config)

	topicName := "manager-high-throughput"
	if err := em.RegisterTopic(topicName); err != nil {
		t.Fatalf("Failed to register topic: %v", err)
	}

	pub, err := em.GetTopicPublisher(topicName)
	if err != nil {
		t.Fatalf("Failed to get publisher: %v", err)
	}

	const numHandlers = 100
	const eventsPerTest = 100000

	handlers := make([]*testHandler, numHandlers)
	for i := 0; i < numHandlers; i++ {
		handlers[i] = &testHandler{id: fmt.Sprintf("handler-%d", i)}
		if _, err := em.AddHandler(topicName, handlers[i]); err != nil {
			t.Fatalf("Failed to add handler: %v", err)
		}
	}

	event := Event{
		Header: [4]byte{'T', 'E', 'S', 'T'},
		Body:   "test-payload",
	}

	start := time.Now()
	for i := 0; i < eventsPerTest; i++ {
		pub.Publish(event)
	}

	time.Sleep(100 * time.Millisecond)

	elapsed := time.Since(start)
	totalExpected := int64(numHandlers * eventsPerTest)
	totalReceived := int64(0)

	for _, h := range handlers {
		received := atomic.LoadInt64(&h.received)
		totalReceived += received
	}

	throughput := float64(eventsPerTest) / elapsed.Seconds()
	t.Logf("EventManager Throughput: %.2f events/sec", throughput)
	t.Logf("Total events published: %d", eventsPerTest)
	t.Logf("Total events received: %d (expected: %d)", totalReceived, totalExpected)
	t.Logf("Time taken: %v", elapsed)

	if totalReceived != totalExpected {
		t.Errorf("Event count mismatch: got %d, expected %d", totalReceived, totalExpected)
	}
}

func TestEventManagerMultiInstance(t *testing.T) {
	ctx := context.Background()

	config1 := &EventManagerConfig{
		MaxTopics:           100,
		MaxHandlersPerTopic: 50,
		WorkerPoolSize:      32,
		Logger:              slog.Default().WithGroup("em1"),
	}

	config2 := &EventManagerConfig{
		MaxTopics:           200,
		MaxHandlersPerTopic: 100,
		WorkerPoolSize:      64,
		Logger:              slog.Default().WithGroup("em2"),
	}

	em1 := NewEventManager(ctx, config1)
	em2 := NewEventManager(ctx, config2)

	if err := em1.RegisterTopic("topic1"); err != nil {
		t.Fatalf("Failed to register topic in em1: %v", err)
	}

	if err := em2.RegisterTopic("topic1"); err != nil {
		t.Fatalf("Failed to register topic in em2: %v", err)
	}

	handler1 := &testHandler{id: "em1-handler"}
	handler2 := &testHandler{id: "em2-handler"}

	if _, err := em1.AddHandler("topic1", handler1); err != nil {
		t.Fatalf("Failed to add handler to em1: %v", err)
	}

	if _, err := em2.AddHandler("topic1", handler2); err != nil {
		t.Fatalf("Failed to add handler to em2: %v", err)
	}

	event := Event{Header: [4]byte{'T', 'S', 'T', '1'}, Body: "test"}

	if err := em1.PublishToTopic("topic1", event); err != nil {
		t.Fatalf("Failed to publish to em1: %v", err)
	}

	if err := em2.PublishToTopic("topic1", event); err != nil {
		t.Fatalf("Failed to publish to em2: %v", err)
	}

	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt64(&handler1.received) != 1 {
		t.Errorf("em1 handler didn't receive event")
	}

	if atomic.LoadInt64(&handler2.received) != 1 {
		t.Errorf("em2 handler didn't receive event")
	}

	t.Log("Multiple EventManager instances work independently")
}

func TestEventManagerConfigurableLimits(t *testing.T) {
	ctx := context.Background()

	config := &EventManagerConfig{
		MaxTopics:           10,
		MaxHandlersPerTopic: 5,
		WorkerPoolSize:      8,
		Logger:              slog.Default(),
	}

	em := NewEventManager(ctx, config)

	for i := 0; i < 10; i++ {
		err := em.RegisterTopic(fmt.Sprintf("topic-%d", i))
		if i < 2 {
			if err != nil {
				t.Errorf("Failed to register topic %d (should succeed): %v", i, err)
			}
		}
	}

	err := em.RegisterTopic("topic-overflow")
	if err == nil {
		t.Error("Should have failed to register topic beyond limit")
	}

	handler := &testHandler{}
	for i := 0; i < 6; i++ {
		_, err := em.AddHandler("topic-0", handler)
		if i < 5 && err != nil {
			t.Errorf("Failed to add handler %d (should succeed): %v", i, err)
		} else if i >= 5 && err == nil {
			t.Error("Should have failed to add handler beyond limit")
		}
	}

	t.Logf("Topic count: %d", em.TopicCount())
	t.Logf("Handler count for topic-0: %d", em.HandlerCount("topic-0"))
}

func BenchmarkEventManagerPublish(b *testing.B) {
	ctx := context.Background()
	config := DefaultEventManagerConfig(slog.Default())
	em := NewEventManager(ctx, config)

	topicName := "benchmark-manager-topic"
	if err := em.RegisterTopic(topicName); err != nil {
		b.Fatalf("Failed to register topic: %v", err)
	}

	pub, err := em.GetTopicPublisher(topicName)
	if err != nil {
		b.Fatalf("Failed to get publisher: %v", err)
	}

	handler := &testHandler{}
	for i := 0; i < 100; i++ {
		if _, err := em.AddHandler(topicName, handler); err != nil {
			b.Fatalf("Failed to add handler: %v", err)
		}
	}

	event := Event{
		Header: [4]byte{'B', 'E', 'N', 'C'},
		Body:   "benchmark-payload",
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pub.Publish(event)
		}
	})
}

func TestEventManagerConcurrentOperations(t *testing.T) {
	ctx := context.Background()
	config := DefaultEventManagerConfig(slog.Default())
	em := NewEventManager(ctx, config)

	const numGoroutines = 100
	const numTopics = 20

	var wg sync.WaitGroup

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()

			topicName := fmt.Sprintf("concurrent-topic-%d", id%numTopics)

			em.RegisterTopic(topicName)

			handler := &testHandler{id: fmt.Sprintf("handler-%d", id)}
			em.AddHandler(topicName, handler)

			if pub, err := em.GetTopicPublisher(topicName); err == nil {
				for j := 0; j < 100; j++ {
					pub.Publish(Event{
						Header: [4]byte{byte(id), byte(j), 0, 0},
						Body:   fmt.Sprintf("msg-%d-%d", id, j),
					})
				}
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	topicCount := em.TopicCount()
	t.Logf("Created %d topics under concurrent load", topicCount)

	userTopics := topicCount
	if userTopics > numTopics {
		t.Errorf("Too many user topics created: %d > %d (total topics: %d, including 8 system topics)",
			userTopics, numTopics, topicCount)
	}
}

func TestEventManagerThrottleThroughput(t *testing.T) {
	ctx := context.Background()
	config := DefaultEventManagerConfig(slog.Default())
	em := NewEventManager(ctx, config)

	topicName := "throttle-throughput-test"
	if err := em.RegisterTopic(topicName); err != nil {
		t.Fatalf("Failed to register topic: %v", err)
	}

	pub, err := em.GetTopicPublisher(topicName)
	if err != nil {
		t.Fatalf("Failed to get publisher: %v", err)
	}

	const testDuration = 10 * time.Second
	const numHandlers = 100

	// Realistic handler that emulates work with random delays
	type realisticHandler struct {
		received int64
		id       string
		minSleep time.Duration
		maxSleep time.Duration
	}

	// Create handlers with different work characteristics
	handlers := make([]*realisticHandler, numHandlers)
	handlerWrappers := make([]EventHandler, numHandlers)

	for i := 0; i < numHandlers; i++ {
		// Mix of fast, medium, and slow handlers
		var minSleep, maxSleep time.Duration
		switch i % 3 {
		case 0: // Fast handlers
			minSleep = 0 * time.Millisecond
			maxSleep = 5 * time.Millisecond
		case 1: // Medium handlers
			minSleep = 5 * time.Millisecond
			maxSleep = 20 * time.Millisecond
		case 2: // Slow handlers
			minSleep = 20 * time.Millisecond
			maxSleep = 50 * time.Millisecond
		}

		handlers[i] = &realisticHandler{
			id:       fmt.Sprintf("handler-%d", i),
			minSleep: minSleep,
			maxSleep: maxSleep,
		}

		// Create proper EventHandler wrapper
		h := handlers[i]
		handlerWrappers[i] = EventHandlerFunc(func(event Event) {
			sleepTime := h.minSleep + time.Duration(rand.Int63n(int64(h.maxSleep-h.minSleep)))
			time.Sleep(sleepTime)
			atomic.AddInt64(&h.received, 1)
		})

		if _, err := em.AddHandler(topicName, handlerWrappers[i]); err != nil {
			t.Fatalf("Failed to add handler: %v", err)
		}
	}

	// Small, medium, and large message sizes
	messageSizes := map[string][]byte{
		"small":  make([]byte, 64),    // 64 bytes
		"medium": make([]byte, 1024),  // 1KB
		"large":  make([]byte, 10240), // 10KB
	}

	// Fill messages with data
	rand.Read(messageSizes["small"])
	rand.Read(messageSizes["medium"])
	rand.Read(messageSizes["large"])

	var eventsPublished int64
	var eventsReceived int64

	start := time.Now()

	// Publisher goroutine with mixed message sizes
	go func() {
		messageTypes := []string{"small", "medium", "large"}
		for {
			if time.Since(start) >= testDuration {
				break
			}

			// Random message size
			msgType := messageTypes[rand.Intn(len(messageTypes))]

			event := Event{
				Header: [4]byte{'S', 'M', 'L', byte(msgType[0])},
				Body:   messageSizes[msgType],
			}

			pub.Publish(event)
			atomic.AddInt64(&eventsPublished, 1)
		}
	}()

	// Wait for the test duration plus buffer for processing
	time.Sleep(testDuration + 500*time.Millisecond)

	// Count received events
	for _, h := range handlers {
		received := atomic.LoadInt64(&h.received)
		atomic.AddInt64(&eventsReceived, received)
	}

	published := atomic.LoadInt64(&eventsPublished)
	received := atomic.LoadInt64(&eventsReceived)

	throughputPublished := float64(published) / testDuration.Seconds()
	throughputReceived := float64(received) / testDuration.Seconds()

	t.Logf("=== Realistic Throttle Throughput Test Results ===")
	t.Logf("Test Duration: %v", testDuration)
	t.Logf("Handlers: %d (mixed fast/medium/slow)", numHandlers)
	t.Logf("Message Sizes: small(64B), medium(1KB), large(10KB)")
	t.Logf("Events Published: %d", published)
	t.Logf("Events Received: %d", received)
	t.Logf("Published Throughput: %.2f events/sec", throughputPublished)
	t.Logf("Received Throughput: %.2f events/sec", throughputReceived)

	if published != received {
		loss := published - received
		lossPercent := float64(loss) / float64(published) * 100
		t.Logf("Event loss: %d events (%.2f%%)", loss, lossPercent)
	}

	// Log per-handler stats
	t.Logf("=== Per-Handler Stats ===")
	for i, h := range handlers {
		received := atomic.LoadInt64(&h.received)
		fastType := "fast"
		if i%3 == 1 {
			fastType = "medium"
		} else if i%3 == 2 {
			fastType = "slow"
		}
		t.Logf("Handler %s (%s): %d events", h.id, fastType, received)
	}
}

// EventHandlerFunc is a function type that implements EventHandler
type EventHandlerFunc func(event Event)

func (f EventHandlerFunc) OnEvent(event Event) {
	f(event)
}

func BenchmarkEventManagerThrottleThroughput(b *testing.B) {
	ctx := context.Background()
	config := DefaultEventManagerConfig(slog.Default())
	em := NewEventManager(ctx, config)

	topicName := "benchmark-throttle"
	if err := em.RegisterTopic(topicName); err != nil {
		b.Fatalf("Failed to register topic: %v", err)
	}

	pub, err := em.GetTopicPublisher(topicName)
	if err != nil {
		b.Fatalf("Failed to get publisher: %v", err)
	}

	handler := &testHandler{}
	if _, err := em.AddHandler(topicName, handler); err != nil {
		b.Fatalf("Failed to add handler: %v", err)
	}

	event := Event{
		Header: [4]byte{'B', 'T', 'H', 'R'},
		Body:   "benchmark-throttle-payload",
	}

	b.ResetTimer()

	var totalEvents int64
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			pub.Publish(event)
			atomic.AddInt64(&totalEvents, 1)
		}
	})

	b.StopTimer()

	// Allow time for all events to be processed
	time.Sleep(100 * time.Millisecond)

	received := atomic.LoadInt64(&handler.received)
	throughput := float64(received) / b.Elapsed().Seconds()

	b.ReportMetric(throughput, "events/sec")
	b.Logf("Total published: %d, Total received: %d", totalEvents, received)
}
