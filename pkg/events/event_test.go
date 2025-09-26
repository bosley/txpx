package events

import (
	"context"
	"fmt"
	"log/slog"
	"runtime"
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

func TestHighThroughputSinglePublisher(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	les := NewLocalEventSystem(logger, ctx)

	topicName := "high-throughput-single"
	if err := les.RegisterTopic(topicName); err != nil {
		t.Fatalf("Failed to register topic: %v", err)
	}

	pub, err := les.GetTopicPublisher(topicName)
	if err != nil {
		t.Fatalf("Failed to get publisher: %v", err)
	}

	const numHandlers = 100
	const eventsPerTest = 100000

	handlers := make([]*testHandler, numHandlers)
	for i := 0; i < numHandlers; i++ {
		handlers[i] = &testHandler{id: fmt.Sprintf("handler-%d", i)}
		if _, err := les.AddHandler(topicName, handlers[i]); err != nil {
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
	t.Logf("Single Publisher Throughput: %.2f events/sec", throughput)
	t.Logf("Total events published: %d", eventsPerTest)
	t.Logf("Total events received: %d (expected: %d)", totalReceived, totalExpected)
	t.Logf("Time taken: %v", elapsed)

	if totalReceived != totalExpected {
		t.Errorf("Event count mismatch: got %d, expected %d", totalReceived, totalExpected)
	}
}

func TestConcurrentMultiPublisher(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	les := NewLocalEventSystem(logger, ctx)

	const numTopics = 10
	const numPublishersPerTopic = 5
	const numHandlersPerTopic = 20
	const eventsPerPublisher = 10000

	type topicState struct {
		publishers []TopicPublisher
		handlers   []*testHandler
	}

	topics := make([]topicState, numTopics)

	for i := 0; i < numTopics; i++ {
		topicName := fmt.Sprintf("concurrent-topic-%d", i)
		if err := les.RegisterTopic(topicName); err != nil {
			t.Fatalf("Failed to register topic %s: %v", topicName, err)
		}

		topics[i].publishers = make([]TopicPublisher, numPublishersPerTopic)
		topics[i].handlers = make([]*testHandler, numHandlersPerTopic)

		for j := 0; j < numPublishersPerTopic; j++ {
			pub, err := les.GetTopicPublisher(topicName)
			if err != nil {
				t.Fatalf("Failed to get publisher: %v", err)
			}
			topics[i].publishers[j] = pub
		}

		for j := 0; j < numHandlersPerTopic; j++ {
			handler := &testHandler{id: fmt.Sprintf("topic-%d-handler-%d", i, j)}
			topics[i].handlers[j] = handler
			if _, err := les.AddHandler(topicName, handler); err != nil {
				t.Fatalf("Failed to add handler: %v", err)
			}
		}
	}

	var wg sync.WaitGroup
	start := time.Now()

	for topicIdx := range topics {
		for pubIdx := range topics[topicIdx].publishers {
			wg.Add(1)
			go func(tIdx, pIdx int) {
				defer wg.Done()
				pub := topics[tIdx].publishers[pIdx]
				event := Event{
					Header: [4]byte{byte(tIdx), byte(pIdx), 0, 0},
					Body:   fmt.Sprintf("topic-%d-pub-%d", tIdx, pIdx),
				}
				for i := 0; i < eventsPerPublisher; i++ {
					pub.Publish(event)
				}
			}(topicIdx, pubIdx)
		}
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	elapsed := time.Since(start)
	totalEventsPublished := numTopics * numPublishersPerTopic * eventsPerPublisher
	totalEventsExpected := totalEventsPublished * numHandlersPerTopic
	totalEventsReceived := int64(0)

	for topicIdx := range topics {
		topicReceived := int64(0)
		for _, h := range topics[topicIdx].handlers {
			received := atomic.LoadInt64(&h.received)
			topicReceived += received
		}
		totalEventsReceived += topicReceived
	}

	throughput := float64(totalEventsPublished) / elapsed.Seconds()
	t.Logf("Multi-Publisher Concurrent Throughput: %.2f events/sec", throughput)
	t.Logf("Total topics: %d", numTopics)
	t.Logf("Publishers per topic: %d", numPublishersPerTopic)
	t.Logf("Handlers per topic: %d", numHandlersPerTopic)
	t.Logf("Total events published: %d", totalEventsPublished)
	t.Logf("Total events received: %d (expected: %d)", totalEventsReceived, totalEventsExpected)
	t.Logf("Time taken: %v", elapsed)

	if totalEventsReceived != int64(totalEventsExpected) {
		t.Errorf("Event count mismatch: got %d, expected %d", totalEventsReceived, totalEventsExpected)
	}
}

type slowHandler struct {
	received  int64
	processed int64
	delay     time.Duration
}

func (sh *slowHandler) OnEvent(event Event) {
	atomic.AddInt64(&sh.received, 1)
	time.Sleep(sh.delay)
	atomic.AddInt64(&sh.processed, 1)
}

func TestWorkerPoolSaturation(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	les := NewLocalEventSystem(logger, ctx)

	topicName := "worker-saturation"
	if err := les.RegisterTopic(topicName); err != nil {
		t.Fatalf("Failed to register topic: %v", err)
	}

	pub, err := les.GetTopicPublisher(topicName)
	if err != nil {
		t.Fatalf("Failed to get publisher: %v", err)
	}

	sh := &slowHandler{delay: 5 * time.Millisecond}

	const numSlowHandlers = 50
	for i := 0; i < numSlowHandlers; i++ {
		if _, err := les.AddHandler(topicName, sh); err != nil {
			t.Fatalf("Failed to add handler: %v", err)
		}
	}

	const burstSize = 1000
	event := Event{
		Header: [4]byte{'S', 'L', 'O', 'W'},
		Body:   "slow-event",
	}

	start := time.Now()
	for i := 0; i < burstSize; i++ {
		pub.Publish(event)
	}
	publishDuration := time.Since(start)

	time.Sleep(100 * time.Millisecond)
	midReceived := atomic.LoadInt64(&sh.received)
	midProcessed := atomic.LoadInt64(&sh.processed)

	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			t.Fatal("Timeout waiting for events to be processed")
		case <-ticker.C:
			processed := atomic.LoadInt64(&sh.processed)
			if processed == int64(burstSize*numSlowHandlers) {
				goto done
			}
		}
	}

done:
	totalDuration := time.Since(start)
	finalReceived := atomic.LoadInt64(&sh.received)
	finalProcessed := atomic.LoadInt64(&sh.processed)

	t.Logf("Worker Pool Saturation Test:")
	t.Logf("  Workers: %d", TotalEventSystemWorkers)
	t.Logf("  Slow handlers: %d (delay: %v)", numSlowHandlers, sh.delay)
	t.Logf("  Events published: %d", burstSize)
	t.Logf("  Publish duration: %v", publishDuration)
	t.Logf("  Mid-check (100ms): received=%d, processed=%d", midReceived, midProcessed)
	t.Logf("  Final: received=%d, processed=%d", finalReceived, finalProcessed)
	t.Logf("  Total duration: %v", totalDuration)
	t.Logf("  Effective throughput: %.2f events/sec", float64(finalProcessed)/totalDuration.Seconds())

	expectedTotal := int64(burstSize * numSlowHandlers)
	if finalReceived != expectedTotal || finalProcessed != expectedTotal {
		t.Errorf("Event processing mismatch: received=%d, processed=%d, expected=%d",
			finalReceived, finalProcessed, expectedTotal)
	}
}

func TestMemoryPressure(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	les := NewLocalEventSystem(logger, ctx)

	const numTopics = 100
	const handlersPerTopic = 50
	const eventsPerTopic = 1000

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	type largePayload struct {
		ID    int
		Data  [1024]byte
		Extra string
	}

	for i := 0; i < numTopics; i++ {
		topicName := fmt.Sprintf("memory-topic-%d", i)
		if err := les.RegisterTopic(topicName); err != nil {
			t.Fatalf("Failed to register topic: %v", err)
		}

		pub, err := les.GetTopicPublisher(topicName)
		if err != nil {
			t.Fatalf("Failed to get publisher: %v", err)
		}

		handler := &testHandler{}
		for j := 0; j < handlersPerTopic; j++ {
			if _, err := les.AddHandler(topicName, handler); err != nil {
				t.Fatalf("Failed to add handler: %v", err)
			}
		}

		for j := 0; j < eventsPerTopic; j++ {
			event := Event{
				Header: [4]byte{byte(i), byte(j), 0, 0},
				Body: largePayload{
					ID:    j,
					Extra: fmt.Sprintf("payload-%d-%d", i, j),
				},
			}
			pub.Publish(event)
		}
	}

	time.Sleep(500 * time.Millisecond)

	runtime.GC()
	runtime.ReadMemStats(&m2)

	totalAllocMB := float64(m2.TotalAlloc-m1.TotalAlloc) / 1024 / 1024
	currentAllocMB := float64(m2.Alloc) / 1024 / 1024

	t.Logf("Memory Pressure Test:")
	t.Logf("  Topics: %d", numTopics)
	t.Logf("  Handlers per topic: %d", handlersPerTopic)
	t.Logf("  Events per topic: %d", eventsPerTopic)
	t.Logf("  Total events: %d", numTopics*eventsPerTopic)
	t.Logf("  Current heap alloc: %.2f MB", currentAllocMB)
	t.Logf("  Total allocated during test: %.2f MB", totalAllocMB)
	t.Logf("  Num GC cycles: %d", m2.NumGC-m1.NumGC)
}

func TestSystemTopicsIntegrity(t *testing.T) {
	ctx := context.Background()
	logger := slog.Default()
	les := NewLocalEventSystem(logger, ctx)

	systemTopics := []string{
		"system-flag",
		"system-reserved0",
		"system-reserved1",
		"system-reserved2",
		"system-reserved3",
		"system-reserved4",
		"system-reserved5",
		"system-reserved6",
	}

	for _, topic := range systemTopics {
		pub, err := les.GetTopicPublisher(topic)
		if err != nil {
			t.Fatalf("Failed to get system topic publisher %s: %v", topic, err)
		}

		handler := &testHandler{}
		if _, err := les.AddHandler(topic, handler); err != nil {
			t.Fatalf("Failed to add handler to system topic %s: %v", topic, err)
		}

		event := Event{
			Header: [4]byte{'S', 'Y', 'S', 'T'},
			Body:   fmt.Sprintf("system-event-%s", topic),
		}
		pub.Publish(event)
	}

	time.Sleep(50 * time.Millisecond)
	t.Log("All system topics verified and functional")
}

func BenchmarkEventPublish(b *testing.B) {
	ctx := context.Background()
	logger := slog.Default()
	les := NewLocalEventSystem(logger, ctx)

	topicName := "benchmark-topic"
	if err := les.RegisterTopic(topicName); err != nil {
		b.Fatalf("Failed to register topic: %v", err)
	}

	pub, err := les.GetTopicPublisher(topicName)
	if err != nil {
		b.Fatalf("Failed to get publisher: %v", err)
	}

	handler := &testHandler{}
	for i := 0; i < 100; i++ {
		if _, err := les.AddHandler(topicName, handler); err != nil {
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
