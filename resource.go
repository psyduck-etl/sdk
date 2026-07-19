package sdk

import (
	"context"
	"fmt"
)

// Resource is the closure-carrying struct plugin authors write. Each
// non-nil Provide* field indicates a role the resource can fulfil; the
// Kinds bitmask must be consistent with which Provide* fields are set.
//
// For each role, a plugin author may set either the context-less Provider
// (backward-compat) or the context-aware ProviderContext (preferred for
// cancellable setup work). If both are set, ProviderContext takes precedence.
type Resource struct {
	Name                      string
	Kinds                     Kind
	Spec                      []*Spec
	ProvideProducer           Provider[Producer]
	ProvideConsumer           Provider[Consumer]
	ProvideTransformer        Provider[Transformer]
	ProvideProducerContext    ProviderContext[Producer]
	ProvideConsumerContext    ProviderContext[Consumer]
	ProvideTransformerContext ProviderContext[Transformer]
}

// NewInProc assembles a Plugin from a name and a set of Resources. It
// handles the Bind kind switch so plugin authors do not repeat it.
func NewInProc(name string, resources ...*Resource) Plugin {
	byName := make(map[string]*Resource, len(resources))
	for _, r := range resources {
		byName[r.Name] = r
	}
	return &inProcPlugin{name: name, resources: resources, byName: byName}
}

type inProcPlugin struct {
	name      string
	resources []*Resource
	byName    map[string]*Resource
}

func (p *inProcPlugin) Name() string { return p.name }

func (p *inProcPlugin) Resources() []ResourceDescriptor {
	out := make([]ResourceDescriptor, len(p.resources))
	for i, r := range p.resources {
		out[i] = ResourceDescriptor{Name: r.Name, Kinds: r.Kinds, Spec: r.Spec}
	}
	return out
}

func (p *inProcPlugin) Bind(ctx context.Context, kind Kind, resource string, block ConfigBlock) (Instance, error) {
	r, ok := p.byName[resource]
	if !ok {
		return nil, fmt.Errorf("plugin %q: unknown resource %q", p.name, resource)
	}

	if !isSingleKind(kind) {
		return nil, fmt.Errorf("plugin %q resource %q: Bind requires a single kind, got %d", p.name, resource, kind)
	}

	if r.Kinds&kind == 0 {
		return nil, fmt.Errorf("plugin %q resource %q: does not support kind %s", p.name, resource, kindName(kind))
	}

	inst := &inProcInstance{resource: r.Name, kind: kind}
	switch kind {
	case PRODUCER:
		if r.ProvideProducerContext != nil {
			fn, err := r.ProvideProducerContext(ctx, block.Decode)
			if err != nil {
				return nil, fmt.Errorf("plugin %q resource %q: build producer: %w", p.name, resource, err)
			}
			inst.produce = fn
		} else if r.ProvideProducer != nil {
			fn, err := r.ProvideProducer(block.Decode)
			if err != nil {
				return nil, fmt.Errorf("plugin %q resource %q: build producer: %w", p.name, resource, err)
			}
			inst.produce = fn
		} else {
			return nil, fmt.Errorf("plugin %q resource %q: no producer provider registered", p.name, resource)
		}
	case CONSUMER:
		if r.ProvideConsumerContext != nil {
			fn, err := r.ProvideConsumerContext(ctx, block.Decode)
			if err != nil {
				return nil, fmt.Errorf("plugin %q resource %q: build consumer: %w", p.name, resource, err)
			}
			inst.consume = fn
		} else if r.ProvideConsumer != nil {
			fn, err := r.ProvideConsumer(block.Decode)
			if err != nil {
				return nil, fmt.Errorf("plugin %q resource %q: build consumer: %w", p.name, resource, err)
			}
			inst.consume = fn
		} else {
			return nil, fmt.Errorf("plugin %q resource %q: no consumer provider registered", p.name, resource)
		}
	case TRANSFORMER:
		if r.ProvideTransformerContext != nil {
			fn, err := r.ProvideTransformerContext(ctx, block.Decode)
			if err != nil {
				return nil, fmt.Errorf("plugin %q resource %q: build transformer: %w", p.name, resource, err)
			}
			inst.transform = fn
		} else if r.ProvideTransformer != nil {
			fn, err := r.ProvideTransformer(block.Decode)
			if err != nil {
				return nil, fmt.Errorf("plugin %q resource %q: build transformer: %w", p.name, resource, err)
			}
			inst.transform = fn
		} else {
			return nil, fmt.Errorf("plugin %q resource %q: no transformer provider registered", p.name, resource)
		}
	}
	return inst, nil
}

func isSingleKind(k Kind) bool {
	return k != 0 && (k&(k-1)) == 0
}

func kindName(k Kind) string {
	switch k {
	case PRODUCER:
		return "producer"
	case CONSUMER:
		return "consumer"
	case TRANSFORMER:
		return "transformer"
	default:
		return fmt.Sprintf("kind(%d)", k)
	}
}

type inProcInstance struct {
	resource  string
	kind      Kind
	produce   Producer
	consume   Consumer
	transform Transformer
}

func (i *inProcInstance) Kind() Kind { return i.kind }

func (i *inProcInstance) Produce(ctx context.Context, send chan<- []byte, errs chan<- error) {
	if i.produce == nil {
		panic(fmt.Sprintf("sdk: resource %q bound as %s, Produce called", i.resource, kindName(i.kind)))
	}
	i.produce(ctx, send, errs)
}

func (i *inProcInstance) Consume(ctx context.Context, recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
	if i.consume == nil {
		panic(fmt.Sprintf("sdk: resource %q bound as %s, Consume called", i.resource, kindName(i.kind)))
	}
	i.consume(ctx, recv, errs, done)
}

func (i *inProcInstance) Transform(ctx context.Context, in <-chan []byte, out chan<- []byte, errs chan<- error) {
	if i.transform == nil {
		panic(fmt.Sprintf("sdk: resource %q bound as %s, Transform called", i.resource, kindName(i.kind)))
	}
	i.transform(ctx, in, out, errs)
}

func (i *inProcInstance) Close() error { return nil }
