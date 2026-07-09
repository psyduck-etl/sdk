package sdk

import (
	"context"
	"sync"
	"testing"
)

func TestProduceFrom(t *testing.T) {
	limit, sent, recv := 10, 0, 0
	send := make(chan []byte)
	next := func() ([]byte, bool, error) {
		if sent == limit {
			return nil, true, nil
		}

		sent++
		return make([]byte, 0), false, nil
	}

	mu := &sync.Mutex{}
	go func() {
		mu.Lock()
		defer mu.Unlock()

		for range send {
			recv++
			if recv == limit {
				return
			}
		}
	}()

	if err := ProduceFrom(context.Background(), next, send); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if recv != limit {
		t.Fatalf("data recv mismatch: %d != %d", recv, limit)
	}

	if sent != recv {
		t.Fatalf("data sent mismatch: %d != %d", sent, recv)
	}
}

func TestConsumeInto(t *testing.T) {
	recvMu := &sync.Mutex{}
	limit, sent, recv := 10, 0, 0
	send := make(chan []byte)
	next := func([]byte) error {
		recvMu.Lock()
		defer recvMu.Unlock()

		recv++
		return nil
	}

	mu := &sync.Mutex{}
	go func() {
		mu.Lock()
		defer mu.Unlock()

		for i := 0; i < limit; i++ {
			sent++
			send <- make([]byte, 0)
		}

		close(send)
	}()

	if err := ConsumeInto(context.Background(), next, send); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if recv != limit {
		t.Fatalf("data recv mismatch: %d != %d", recv, limit)
	}

	if sent != recv {
		t.Fatalf("data sent mismatch: %d != %d", sent, recv)
	}
}

func TestProduceFromContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	sent := 0
	send := make(chan []byte, 10)
	next := func() ([]byte, bool, error) {
		sent++
		if sent >= 100 {
			return nil, true, nil
		}
		return make([]byte, sent), false, nil
	}

	go func() {
		// Cancel after 3 items have been sent
		for i := 0; i < 3; i++ {
			<-send
		}
		cancel()
	}()

	err := ProduceFrom(ctx, next, send)
	if err != context.Canceled {
		t.Fatalf("ProduceFrom returned %v, want context.Canceled", err)
	}
	if sent >= 100 {
		t.Fatalf("ProduceFrom did not stop on context cancellation; sent %d items", sent)
	}
}

func TestConsumeIntoContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	recv := make(chan []byte)
	processed := 0
	processingMu := &sync.Mutex{}
	processingCV := sync.NewCond(processingMu)

	next := func([]byte) error {
		processingMu.Lock()
		processed++
		processingCV.Broadcast()
		processingMu.Unlock()
		return nil
	}

	// Send items in a goroutine, cancel after 3 are processed
	go func() {
		for i := 0; i < 100; i++ {
			recv <- make([]byte, 0)
			// Give consumer time to process
		}
	}()

	go func() {
		// Wait until 3 items have been processed, then cancel
		processingMu.Lock()
		for processed < 3 {
			processingCV.Wait()
		}
		processingMu.Unlock()
		cancel()
	}()

	err := ConsumeInto(ctx, next, recv)
	if err != context.Canceled {
		t.Fatalf("ConsumeInto returned %v, want context.Canceled", err)
	}
	// At least 3 should have been processed before cancellation
	if processed < 3 {
		t.Fatalf("ConsumeInto processed %d items, want at least 3", processed)
	}
}
