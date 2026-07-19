package sdk

import (
	"context"
	"errors"
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

func TestMapContext(t *testing.T) {
	limit := 10
	in := make(chan []byte)
	out := make(chan []byte)
	errs := make(chan error, 10)

	fn := func(_ context.Context, data []byte) ([]byte, error) {
		return data, nil
	}

	transformer := MapContext(fn)
	go transformer(context.Background(), in, out, errs)

	go func() {
		for i := 0; i < limit; i++ {
			in <- make([]byte, 0)
		}
		close(in)
	}()

	recv := 0
	for range out {
		recv++
	}

	if recv != limit {
		t.Fatalf("data recv mismatch: %d != %d", recv, limit)
	}
}

func TestMapContextFiltering(t *testing.T) {
	in := make(chan []byte)
	out := make(chan []byte)
	errs := make(chan error, 10)

	fn := func(_ context.Context, data []byte) ([]byte, error) {
		// Filter out every other item
		if len(data) > 0 && data[0]%2 == 0 {
			return nil, nil
		}
		return data, nil
	}

	transformer := MapContext(fn)
	go transformer(context.Background(), in, out, errs)

	// Send 10 items, only odd indices pass through
	go func() {
		for i := 0; i < 10; i++ {
			in <- []byte{byte(i)}
		}
		close(in)
	}()

	recv := 0
	for range out {
		recv++
	}

	// Should only receive 5 items (indices 1, 3, 5, 7, 9)
	if recv != 5 {
		t.Fatalf("filtered recv count mismatch: %d, want 5", recv)
	}
}

func TestMapContextErrorHandling(t *testing.T) {
	in := make(chan []byte)
	out := make(chan []byte)
	errs := make(chan error, 10)

	testErr := errors.New("test error")
	callCount := 0
	fn := func(_ context.Context, data []byte) ([]byte, error) {
		callCount++
		if callCount == 2 {
			return nil, testErr
		}
		return data, nil
	}

	transformer := MapContext(fn)
	go transformer(context.Background(), in, out, errs)

	// Send 5 items, second one will error
	go func() {
		for i := 0; i < 5; i++ {
			in <- []byte{byte(i)}
		}
		close(in)
	}()

	recv := 0
	errRecv := 0
	for {
		select {
		case _, ok := <-out:
			if !ok {
				goto done
			}
			recv++
		case err := <-errs:
			if err == testErr {
				errRecv++
			}
		}
	}
done:

	if recv != 4 {
		t.Fatalf("recv count mismatch: %d, want 4", recv)
	}
	if errRecv != 1 {
		t.Fatalf("error recv count mismatch: %d, want 1", errRecv)
	}
}

func TestMapContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []byte, 100)
	out := make(chan []byte, 10)
	errs := make(chan error, 10)

	sentMu := &sync.Mutex{}
	sent := 0
	fn := func(_ context.Context, data []byte) ([]byte, error) {
		sentMu.Lock()
		sent++
		sentMu.Unlock()
		return data, nil
	}

	transformer := MapContext(fn)
	go transformer(ctx, in, out, errs)

	// Send items in a goroutine
	go func() {
		for i := 0; i < 100; i++ {
			in <- []byte{byte(i)}
		}
	}()

	// Receive a few and cancel
	recv := 0
	for i := 0; i < 3; i++ {
		<-out
		recv++
	}
	cancel()

	// Wait for out to close
	for range out {
		recv++
	}

	sentMu.Lock()
	defer sentMu.Unlock()
	if recv < 3 {
		t.Fatalf("recv count too low: %d, want at least 3", recv)
	}
	if sent >= 100 {
		t.Fatalf("sent all items despite cancellation: %d", sent)
	}
}

func TestMapContextPropagatesContext(t *testing.T) {
	key := "test-key"
	value := "test-value"
	ctx := context.WithValue(context.Background(), key, value)
	in := make(chan []byte)
	out := make(chan []byte)
	errs := make(chan error, 10)

	receivedValue := ""
	fn := func(fnCtx context.Context, data []byte) ([]byte, error) {
		if v := fnCtx.Value(key); v != nil {
			receivedValue = v.(string)
		}
		return data, nil
	}

	transformer := MapContext(fn)
	go transformer(ctx, in, out, errs)

	go func() {
		in <- []byte{1}
		close(in)
	}()

	for range out {
	}

	if receivedValue != value {
		t.Fatalf("context value mismatch: %q, want %q", receivedValue, value)
	}
}

func TestMapPreservesSemantics(t *testing.T) {
	limit := 10
	in := make(chan []byte)
	out := make(chan []byte)
	errs := make(chan error, 10)

	fn := func(data []byte) ([]byte, error) {
		return data, nil
	}

	transformer := Map(fn)
	go transformer(context.Background(), in, out, errs)

	go func() {
		for i := 0; i < limit; i++ {
			in <- make([]byte, 0)
		}
		close(in)
	}()

	recv := 0
	for range out {
		recv++
	}

	if recv != limit {
		t.Fatalf("Map semantics changed: %d != %d", recv, limit)
	}
}
