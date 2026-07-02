package sdk

import (
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
			return func(send chan<- []byte, errs chan<- error) {
				send <- []byte(cfg.Prefix)
				close(send)
			}, nil
		},
		ProvideConsumer: func(parse Parser) (Consumer, error) {
			return func(recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
				for range recv {
				}
				done <- struct{}{}
			}, nil
		},
		ProvideTransformer: func(parse Parser) (Transformer, error) {
			return func(in []byte) ([]byte, error) { return in, nil }, nil
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
	inst.Produce(send, errs)
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
	go inst.Consume(recv, errs, done)
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
	out, err := inst.Transform([]byte("x"))
	if err != nil || string(out) != "x" {
		t.Fatalf("Transform = %q,%v; want %q,nil", out, err, "x")
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
			return func(send chan<- []byte, errs chan<- error) {}, nil
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
		inst.Consume(nil, nil, nil)
	}()

	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("Transform on PRODUCER instance did not panic")
			}
		}()
		_, _ = inst.Transform(nil)
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
