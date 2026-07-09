package sdk

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// stubBlock is a minimal ConfigBlock backed by a JSON blob. Decode
// unmarshals into dst; this lets tests populate arbitrary struct fields
// without depending on a config format.
type stubBlock struct {
	origin SourceRange
	data   []byte
	err    error
}

func (b *stubBlock) Origin() SourceRange { return b.origin }
func (b *stubBlock) Encode() ([]byte, error) {
	if b.err != nil {
		return nil, b.err
	}
	if len(b.data) == 0 {
		return []byte("{}"), nil
	}
	return b.data, nil
}
func (b *stubBlock) Decode(dst any) error {
	if b.err != nil {
		return b.err
	}
	if len(b.data) == 0 {
		return nil
	}
	return json.Unmarshal(b.data, dst)
}

type producerCfg struct {
	Prefix string `json:"prefix"`
}

func newTestResource() *Resource {
	return &Resource{
		Name:  "widget",
		Kinds: PRODUCER | CONSUMER | TRANSFORMER,
		Spec:  []*Spec{{Name: "prefix", Type: TypeString}},
		ProvideProducer: func(parse Parser) (Producer, error) {
			cfg := &producerCfg{}
			if err := parse(cfg); err != nil {
				return nil, err
			}
			return func(ctx context.Context, send chan<- []byte, errs chan<- error) {
				send <- []byte(cfg.Prefix)
				close(send)
			}, nil
		},
		ProvideConsumer: func(parse Parser) (Consumer, error) {
			return func(ctx context.Context, recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
				for range recv {
				}
				done <- struct{}{}
			}, nil
		},
		ProvideTransformer: func(parse Parser) (Transformer, error) {
			return func(ctx context.Context, in <-chan []byte, out chan<- []byte, errs chan<- error) {
				defer close(out)
				for data := range in {
					select {
					case out <- data:
					case <-ctx.Done():
						return
					}
				}
			}, nil
		},
	}
}

func TestNewInProcNameAndResources(t *testing.T) {
	r := newTestResource()
	p := NewInProc("test-plugin", r)

	if p.Name() != "test-plugin" {
		t.Fatalf("Name = %q, want %q", p.Name(), "test-plugin")
	}

	descs := p.Resources()
	if len(descs) != 1 {
		t.Fatalf("Resources len = %d, want 1", len(descs))
	}
	if descs[0].Name != "widget" || descs[0].Kinds != (PRODUCER|CONSUMER|TRANSFORMER) {
		t.Fatalf("descriptor mismatch: %+v", descs[0])
	}
	if len(descs[0].Spec) != 1 || descs[0].Spec[0].Name != "prefix" {
		t.Fatalf("spec projection mismatch: %+v", descs[0].Spec)
	}
}

func TestBindHappyPath_Producer(t *testing.T) {
	p := NewInProc("pl", newTestResource())
	block := &stubBlock{data: []byte(`{"prefix":"hi"}`)}
	inst, err := p.Bind(PRODUCER, "widget", block)
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if inst.Kind() != PRODUCER {
		t.Fatalf("Kind = %d, want PRODUCER", inst.Kind())
	}
	send := make(chan []byte, 1)
	errs := make(chan error, 1)
	inst.Produce(context.Background(), send, errs)
	got := <-send
	if string(got) != "hi" {
		t.Fatalf("Produce sent %q, want %q", got, "hi")
	}
	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestBindHappyPath_Consumer(t *testing.T) {
	p := NewInProc("pl", newTestResource())
	inst, err := p.Bind(CONSUMER, "widget", &stubBlock{})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if inst.Kind() != CONSUMER {
		t.Fatalf("Kind = %d, want CONSUMER", inst.Kind())
	}
	recv := make(chan []byte)
	errs := make(chan error, 1)
	done := make(chan struct{}, 1)
	go inst.Consume(context.Background(), recv, errs, done)
	close(recv)
	<-done
}

func TestBindHappyPath_Transformer(t *testing.T) {
	p := NewInProc("pl", newTestResource())
	inst, err := p.Bind(TRANSFORMER, "widget", &stubBlock{})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if inst.Kind() != TRANSFORMER {
		t.Fatalf("Kind = %d, want TRANSFORMER", inst.Kind())
	}
	in := make(chan []byte, 1)
	out := make(chan []byte, 1)
	errs := make(chan error, 1)
	in <- []byte("x")
	close(in)
	inst.Transform(context.Background(), in, out, errs)
	got, ok := <-out
	if !ok || string(got) != "x" {
		t.Fatalf("Transform out = %q,%v; want %q,true", got, ok, "x")
	}
	if _, ok := <-out; ok {
		t.Fatal("expected out to be closed")
	}
}

func TestBindErrors(t *testing.T) {
	p := NewInProc("pl", newTestResource())
	block := &stubBlock{}

	if _, err := p.Bind(PRODUCER, "nope", block); err == nil {
		t.Fatal("expected error for unknown resource")
	}

	if _, err := p.Bind(PRODUCER|CONSUMER, "widget", block); err == nil {
		t.Fatal("expected error for multi-kind Bind")
	}

	if _, err := p.Bind(Kind(0), "widget", block); err == nil {
		t.Fatal("expected error for zero-kind Bind")
	}

	rProducerOnly := &Resource{
		Name:            "only-p",
		Kinds:           PRODUCER,
		ProvideProducer: func(Parser) (Producer, error) { return nil, nil },
	}
	pp := NewInProc("pl", rProducerOnly)
	if _, err := pp.Bind(CONSUMER, "only-p", block); err == nil {
		t.Fatal("expected error binding kind not in resource.Kinds")
	}

	rNilProvider := &Resource{
		Name:  "nil-c",
		Kinds: PRODUCER | CONSUMER,
		ProvideProducer: func(Parser) (Producer, error) {
			return func(ctx context.Context, send chan<- []byte, errs chan<- error) {}, nil
		},
		// ProvideConsumer intentionally nil despite CONSUMER in Kinds.
	}
	pn := NewInProc("pl", rNilProvider)
	if _, err := pn.Bind(CONSUMER, "nil-c", block); err == nil {
		t.Fatal("expected error when Provide* is nil")
	}
}

func TestBindProviderErrorPropagates(t *testing.T) {
	wantErr := errors.New("boom")
	r := &Resource{
		Name:  "boom",
		Kinds: PRODUCER,
		ProvideProducer: func(Parser) (Producer, error) {
			return nil, wantErr
		},
	}
	p := NewInProc("pl", r)
	if _, err := p.Bind(PRODUCER, "boom", &stubBlock{}); err == nil || !errors.Is(err, wantErr) {
		t.Fatalf("Bind err = %v, want wrap of %v", err, wantErr)
	}
}

func TestInstancePanicsOnWrongKind(t *testing.T) {
	p := NewInProc("pl", newTestResource())
	inst, err := p.Bind(PRODUCER, "widget", &stubBlock{data: []byte(`{"prefix":""}`)})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("Consume on PRODUCER instance did not panic")
			}
		}()
		inst.Consume(context.Background(), nil, nil, nil)
	}()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("Transform on PRODUCER instance did not panic")
			}
		}()
		inst.Transform(context.Background(), nil, nil, nil)
	}()
}

func TestSourceRangeString(t *testing.T) {
	if got := (SourceRange{}).String(); got != "<unknown>" {
		t.Fatalf("zero SourceRange = %q, want %q", got, "<unknown>")
	}
	r := SourceRange{SourceName: "pipeline.psy", StartLine: 12, StartCol: 3, EndLine: 14, EndCol: 1}
	if got := r.String(); got != "pipeline.psy:12:3-14:1" {
		t.Fatalf("SourceRange = %q, want %q", got, "pipeline.psy:12:3-14:1")
	}
}

func TestInstanceProducerContextCancellation(t *testing.T) {
	// Create a resource with a producer that respects context cancellation
	r := &Resource{
		Name:  "cancellable-producer",
		Kinds: PRODUCER,
		ProvideProducer: func(Parser) (Producer, error) {
			return func(ctx context.Context, send chan<- []byte, errs chan<- error) {
				defer close(send)
				for i := 0; i < 1000; i++ {
					select {
					case send <- []byte("data"):
					case <-ctx.Done():
						errs <- ctx.Err()
						return
					}
				}
			}, nil
		},
	}
	p := NewInProc("pl", r)
	inst, err := p.Bind(PRODUCER, "cancellable-producer", &stubBlock{})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	send := make(chan []byte, 10)
	errs := make(chan error, 1)

	go func() {
		// Receive a few items then cancel
		for i := 0; i < 3; i++ {
			<-send
		}
		cancel()
	}()

	inst.Produce(ctx, send, errs)

	// Should have gotten a context.Canceled error
	if err := <-errs; err != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

func TestInstanceConsumerContextCancellation(t *testing.T) {
	// Create a resource with a consumer that respects context cancellation
	r := &Resource{
		Name:  "cancellable-consumer",
		Kinds: CONSUMER,
		ProvideConsumer: func(Parser) (Consumer, error) {
			return func(ctx context.Context, recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
				itemCount := 0
				for {
					select {
					case data, ok := <-recv:
						if !ok {
							done <- struct{}{}
							return
						}
						_ = data
						itemCount++
					case <-ctx.Done():
						errs <- ctx.Err()
						return
					}
				}
			}, nil
		},
	}
	p := NewInProc("pl", r)
	inst, err := p.Bind(CONSUMER, "cancellable-consumer", &stubBlock{})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	recv := make(chan []byte, 10)
	errs := make(chan error, 1)
	done := make(chan struct{}, 1)

	go func() {
		inst.Consume(ctx, recv, errs, done)
	}()

	// Send a few items
	for i := 0; i < 3; i++ {
		recv <- []byte("data")
	}

	// Cancel and verify consumer exits with context.Canceled
	cancel()

	// Should get context.Canceled on errs, not done (which means natural close)
	select {
	case err := <-errs:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-done:
		t.Fatal("consumer closed normally instead of exiting on context cancellation")
	}
}

func TestInstanceTransformerContextCancellation(t *testing.T) {
	// Create a resource with a transformer that respects context cancellation
	r := &Resource{
		Name:  "cancellable-transformer",
		Kinds: TRANSFORMER,
		ProvideTransformer: func(Parser) (Transformer, error) {
			return func(ctx context.Context, in <-chan []byte, out chan<- []byte, errs chan<- error) {
				defer close(out)
				for {
					select {
					case data, ok := <-in:
						if !ok {
							return
						}
						select {
						case out <- data:
						case <-ctx.Done():
							errs <- ctx.Err()
							return
						}
					case <-ctx.Done():
						errs <- ctx.Err()
						return
					}
				}
			}, nil
		},
	}
	p := NewInProc("pl", r)
	inst, err := p.Bind(TRANSFORMER, "cancellable-transformer", &stubBlock{})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	in := make(chan []byte, 10)
	out := make(chan []byte)
	errs := make(chan error, 1)

	go inst.Transform(ctx, in, out, errs)

	in <- []byte("data")
	cancel()

	select {
	case err := <-errs:
		if err != context.Canceled {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	case <-out:
		t.Fatal("did not expect out to be readable after cancellation with no reader")
	}
}
