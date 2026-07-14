package rpc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	goplugin "github.com/hashicorp/go-plugin"

	"github.com/psyduck-etl/sdk"
)

// testPlugin assembles the in-proc plugin the round-trip tests serve. Its
// resources echo enough state (config values, kind dispatch, error paths)
// to prove the wire preserves the sdk contracts.
func testPlugin() sdk.Plugin {
	return sdk.NewInProc("round-trip",
		&sdk.Resource{
			Name:  "emit",
			Kinds: sdk.PRODUCER,
			Spec: []*sdk.Spec{
				{Name: "value", Type: sdk.TypeString, Required: true},
				{Name: "count", Type: sdk.TypeInt, Default: 1},
				{Name: "fail-with", Type: sdk.TypeString},
			},
			ProvideProducer: func(parse sdk.Parser) (sdk.Producer, error) {
				cfg := struct {
					Value    string `psy:"value"`
					Count    int    `psy:"count"`
					FailWith string `psy:"fail-with"`
				}{}
				if err := parse(&cfg); err != nil {
					return nil, err
				}
				return func(ctx context.Context, send chan<- []byte, errs chan<- error) {
					defer close(send)
					for i := range cfg.Count {
						select {
						case send <- fmt.Appendf(nil, "%s-%d", cfg.Value, i):
						case <-ctx.Done():
							errs <- ctx.Err()
							return
						}
					}
					if cfg.FailWith != "" {
						errs <- errors.New(cfg.FailWith)
					}
				}, nil
			},
		},
		&sdk.Resource{
			Name:  "forever",
			Kinds: sdk.PRODUCER,
			ProvideProducer: func(parse sdk.Parser) (sdk.Producer, error) {
				return func(ctx context.Context, send chan<- []byte, errs chan<- error) {
					defer close(send)
					for {
						select {
						case send <- []byte("tick"):
						case <-ctx.Done():
							errs <- ctx.Err()
							return
						}
					}
				}, nil
			},
		},
		&sdk.Resource{
			Name:  "collect",
			Kinds: sdk.CONSUMER,
			Spec:  []*sdk.Spec{{Name: "abort-on", Type: sdk.TypeString}},
			ProvideConsumer: func(parse sdk.Parser) (sdk.Consumer, error) {
				cfg := struct {
					AbortOn string `psy:"abort-on"`
				}{}
				if err := parse(&cfg); err != nil {
					return nil, err
				}
				return func(ctx context.Context, recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
					count := 0
					for {
						select {
						case b, ok := <-recv:
							if !ok {
								// Normal completion: report the tally as an
								// error event (visible to the test), close
								// errs, signal done.
								errs <- fmt.Errorf("consumed %d", count)
								close(errs)
								done <- struct{}{}
								return
							}
							if cfg.AbortOn != "" && string(b) == cfg.AbortOn {
								errs <- fmt.Errorf("aborted on %s", b)
								return
							}
							count++
						case <-ctx.Done():
							errs <- ctx.Err()
							return
						}
					}
				}, nil
			},
		},
		&sdk.Resource{
			Name:  "suffix",
			Kinds: sdk.TRANSFORMER,
			Spec:  []*sdk.Spec{{Name: "suffix", Type: sdk.TypeString, Default: "!"}},
			ProvideTransformer: func(parse sdk.Parser) (sdk.Transformer, error) {
				cfg := struct {
					Suffix string `psy:"suffix"`
				}{}
				if err := parse(&cfg); err != nil {
					return nil, err
				}
				return func(in []byte) ([]byte, error) {
					switch string(in) {
					case "drop-me":
						return nil, nil
					case "fail-me":
						return nil, errors.New("transform failed")
					case "empty-me":
						return []byte{}, nil
					}
					return append(in, cfg.Suffix...), nil
				}, nil
			},
		},
	)
}

// dispense runs the Driver service over go-plugin's in-process gRPC pair
// and hands back the host-facing sdk.Plugin.
func dispense(t *testing.T) sdk.Plugin {
	t.Helper()
	client, _ := goplugin.TestPluginGRPCConn(t, false, map[string]goplugin.Plugin{
		pluginName: &driverPlugin{impl: testPlugin()},
	})
	t.Cleanup(func() { client.Close() })

	raw, err := client.Dispense(pluginName)
	if err != nil {
		t.Fatalf("Dispense: %v", err)
	}
	p, ok := raw.(sdk.Plugin)
	if !ok {
		t.Fatalf("Dispense returned %T, want sdk.Plugin", raw)
	}
	return p
}

func block(t *testing.T, config string) sdk.ConfigBlock {
	t.Helper()
	return sdk.NewJSONBlock(sdk.SourceRange{SourceName: "rpc_test.psy", StartLine: 1, StartCol: 1, EndLine: 1, EndCol: 2}, []byte(config))
}

func TestSchemaRoundTrip(t *testing.T) {
	p := dispense(t)

	if p.Name() != "round-trip" {
		t.Errorf("Name = %q", p.Name())
	}

	byName := map[string]sdk.ResourceDescriptor{}
	for _, r := range p.Resources() {
		byName[r.Name] = r
	}
	if len(byName) != 4 {
		t.Fatalf("Resources = %d, want 4", len(byName))
	}

	emit := byName["emit"]
	if emit.Kinds != sdk.PRODUCER {
		t.Errorf("emit.Kinds = %d", emit.Kinds)
	}
	if len(emit.Spec) != 3 {
		t.Fatalf("emit.Spec = %d entries", len(emit.Spec))
	}
	specs := map[string]*sdk.Spec{}
	for _, s := range emit.Spec {
		specs[s.Name] = s
	}
	if !specs["value"].Required || specs["value"].Type != sdk.TypeString {
		t.Errorf("value spec = %+v", specs["value"])
	}
	if got := specs["count"].Default; got != int64(1) {
		t.Errorf("count default = %#v, want int64(1) after the wire", got)
	}
	if def := byName["suffix"].Spec[0].Default; def != "!" {
		t.Errorf("suffix default = %#v", def)
	}
}

func TestBindErrors(t *testing.T) {
	p := dispense(t)

	if _, err := p.Bind(sdk.PRODUCER, "no-such", block(t, `{}`)); err == nil || !strings.Contains(err.Error(), "no-such") {
		t.Errorf("Bind unknown resource: %v", err)
	}
	if _, err := p.Bind(sdk.CONSUMER, "emit", block(t, `{}`)); err == nil || !strings.Contains(err.Error(), "consumer") {
		t.Errorf("Bind wrong kind: %v", err)
	}
}

func TestProduceRoundTrip(t *testing.T) {
	p := dispense(t)

	inst, err := p.Bind(sdk.PRODUCER, "emit", block(t, `{"value": "msg", "count": 3, "fail-with": "boom"}`))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if inst.Kind() != sdk.PRODUCER {
		t.Errorf("Kind = %d", inst.Kind())
	}

	send := make(chan []byte)
	errs := make(chan error, 4)
	go inst.Produce(t.Context(), send, errs)

	var got []string
	deadline := time.After(5 * time.Second)
Loop:
	for {
		select {
		case b, ok := <-send:
			if !ok {
				break Loop
			}
			got = append(got, string(b))
		case <-deadline:
			t.Fatalf("timeout after %d messages", len(got))
		}
	}

	want := []string{"msg-0", "msg-1", "msg-2"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("message %d = %q, want %q", i, got[i], want[i])
		}
	}

	select {
	case err := <-errs:
		if err == nil || err.Error() != "boom" {
			t.Errorf("error = %v, want boom", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for producer error")
	}
}

func TestProduceCancel(t *testing.T) {
	p := dispense(t)

	inst, err := p.Bind(sdk.PRODUCER, "forever", block(t, `{}`))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	send := make(chan []byte)
	errs := make(chan error, 1)
	go inst.Produce(ctx, send, errs)

	for range 3 {
		select {
		case <-send:
		case <-time.After(5 * time.Second):
			t.Fatal("timeout waiting for ticks")
		}
	}
	cancel()

	deadline := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-send:
			if !ok {
				return // send closed after cancellation: the contract held
			}
		case <-deadline:
			t.Fatal("send not closed after cancel")
		}
	}
}

func TestConsumeRoundTrip(t *testing.T) {
	p := dispense(t)

	inst, err := p.Bind(sdk.CONSUMER, "collect", block(t, `{}`))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	recv := make(chan []byte)
	errs := make(chan error, 4)
	done := make(chan struct{}, 1)
	go inst.Consume(t.Context(), recv, errs, done)

	for i := range 5 {
		select {
		case recv <- fmt.Appendf(nil, "item-%d", i):
		case <-time.After(5 * time.Second):
			t.Fatal("timeout feeding consumer")
		}
	}
	close(recv)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for done")
	}

	// The consumer reported its tally on errs before closing it; the relay
	// must deliver both the error and the close.
	select {
	case err, ok := <-errs:
		if !ok || err == nil || err.Error() != "consumed 5" {
			t.Errorf("tally = %v (ok=%v), want consumed 5", err, ok)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for tally")
	}
	if _, ok := <-errs; ok {
		t.Error("errs should be closed after normal completion")
	}
}

func TestConsumeAbort(t *testing.T) {
	p := dispense(t)

	inst, err := p.Bind(sdk.CONSUMER, "collect", block(t, `{"abort-on": "poison"}`))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	recv := make(chan []byte)
	errs := make(chan error, 4)
	done := make(chan struct{}, 1)
	consumeReturned := make(chan struct{})
	go func() {
		defer close(consumeReturned)
		inst.Consume(t.Context(), recv, errs, done)
	}()

	recv <- []byte("fine")
	recv <- []byte("poison")

	select {
	case err := <-errs:
		if err == nil || !strings.Contains(err.Error(), "aborted on poison") {
			t.Errorf("error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for abort error")
	}

	close(recv) // teardown: the relay's writer drains and half-closes
	select {
	case <-consumeReturned:
	case <-time.After(5 * time.Second):
		t.Fatal("Consume did not return after abort")
	}

	select {
	case <-done:
		t.Error("done signaled for an aborted consumer")
	default:
	}
}

func TestTransformRoundTrip(t *testing.T) {
	p := dispense(t)

	inst, err := p.Bind(sdk.TRANSFORMER, "suffix", block(t, `{"suffix": "?"}`))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}

	out, err := inst.Transform([]byte("hello"))
	if err != nil || string(out) != "hello?" {
		t.Errorf("Transform = %q, %v", out, err)
	}

	out, err = inst.Transform([]byte("drop-me"))
	if err != nil || out != nil {
		t.Errorf("drop: Transform = %#v, %v — want nil, nil", out, err)
	}

	out, err = inst.Transform([]byte("empty-me"))
	if err != nil || out == nil || len(out) != 0 {
		t.Errorf("empty: Transform = %#v, %v — want non-nil empty", out, err)
	}

	if _, err = inst.Transform([]byte("fail-me")); err == nil || err.Error() != "transform failed" {
		t.Errorf("fail: err = %v", err)
	}
}

func TestCloseInvalidatesHandle(t *testing.T) {
	p := dispense(t)

	inst, err := p.Bind(sdk.TRANSFORMER, "suffix", block(t, `{}`))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if err := inst.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := inst.Transform([]byte("x")); err == nil {
		t.Error("Transform after Close should error")
	}
	if err := inst.Close(); err == nil {
		t.Error("double Close should error (handle already released)")
	}
}

func TestKindMismatchPanics(t *testing.T) {
	p := dispense(t)

	inst, err := p.Bind(sdk.TRANSFORMER, "suffix", block(t, `{}`))
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Error("Produce on a transformer instance should panic")
		}
	}()
	inst.Produce(t.Context(), make(chan []byte), make(chan error))
}
