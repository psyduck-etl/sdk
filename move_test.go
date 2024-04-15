package sdk

import (
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

	if err := ProduceFrom(next, send); err != nil {
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

	if err := ConsumeInto(next, ConsumeIntoConfig{0}, send); err != nil {
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
