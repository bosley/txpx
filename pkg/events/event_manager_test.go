package events

import (
	"context"
	"fmt"
	"log/slog"
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

	userTopics := topicCount - 8 // sys topics
	if userTopics > numTopics {
		t.Errorf("Too many user topics created: %d > %d (total topics: %d, including 8 system topics)",
			userTopics, numTopics, topicCount)
	}
}
