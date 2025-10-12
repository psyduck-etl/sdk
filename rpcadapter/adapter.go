package rpcadapter

import (
	"context"

	"github.com/psyduck-etl/sdk"
	"github.com/psyduck-etl/sdk/rpc"
)

// Adapter lets an sdk.Plugin be served by the rpc.Server without introducing an import cycle.
type Adapter struct {
	plugin *sdk.Plugin
}

func New(plugin *sdk.Plugin) *Adapter {
	return &Adapter{plugin: plugin}
}

func (a *Adapter) Resources() []rpc.Resource {
	out := make([]rpc.Resource, 0, len(a.plugin.Resources))
	for _, r := range a.plugin.Resources {
		res := rpc.Resource{Name: r.Name}
		if r.Kinds&sdk.PRODUCER != 0 {
			res.Kind = "PRODUCER"
		}
		if r.Kinds&sdk.CONSUMER != 0 {
			res.Kind = "CONSUMER"
		}
		if r.Kinds&sdk.TRANSFORMER != 0 {
			res.Kind = "TRANSFORMER"
		}

		// capture loop variable
		rLocal := r
		res.ProvideProducer = func(parse func(interface{}) error) (rpc.Producer, error) {
			if rLocal.ProvideProducer == nil {
				return nil, nil
			}
			prod, err := rLocal.ProvideProducer.ProvideProducer(parse)
			if err != nil {
				return nil, err
			}
			// adapt sdk.Producer to rpc.Producer
			return &producerAdapter{prod}, nil
		}
		res.ProvideConsumer = func(parse func(interface{}) error) (rpc.Consumer, error) {
			if rLocal.ProvideConsumer == nil {
				return nil, nil
			}
			cons, err := rLocal.ProvideConsumer.ProvideConsumer(parse)
			if err != nil {
				return nil, err
			}
			return &consumerAdapter{cons}, nil
		}
		res.ProvideTransformer = func(parse func(interface{}) error) (rpc.Transformer, error) {
			if rLocal.ProvideTransformer == nil {
				return nil, nil
			}
			tr, err := rLocal.ProvideTransformer.ProvideTransformer(parse)
			if err != nil {
				return nil, err
			}
			return &transformerAdapter{tr}, nil
		}
		out = append(out, res)
	}
	return out
}

type producerAdapter struct{ p sdk.Producer }

func (a *producerAdapter) Start(ctx context.Context, send func([]byte) error) error {
	return a.p.Start(ctx, send)
}
func (a *producerAdapter) Stop() error { return a.p.Stop() }

type consumerAdapter struct{ c sdk.Consumer }

func (a *consumerAdapter) Consume(ctx context.Context, recv func() ([]byte, error)) error {
	return a.c.Consume(ctx, recv)
}
func (a *consumerAdapter) Stop() error { return a.c.Stop() }

type transformerAdapter struct{ t sdk.Transformer }

func (a *transformerAdapter) Transform(in []byte) ([]byte, error) { return a.t.Transform(in) }
