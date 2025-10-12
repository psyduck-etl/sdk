package sdk

import (
	"context"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// MoverKind represents the type of data processing capability a resource provides.
// Resources can support multiple kinds using bitwise OR operations.
type MoverKind uint64

const (
	// PRODUCER indicates a resource that generates data messages
	PRODUCER = MoverKind(1 << iota)

	// CONSUMER indicates a resource that consumes and processes data messages
	CONSUMER

	// TRANSFORMER indicates a resource that transforms data messages
	TRANSFORMER
)

// Resource defines a single data processing capability provided by a plugin.
// Resources encapsulate the logic for producing, consuming, or transforming data
// along with their configuration specifications.
type Resource struct {
	// Kinds specifies what capabilities this resource provides (PRODUCER, CONSUMER, TRANSFORMER)
	// Multiple kinds can be combined using bitwise OR: PRODUCER | TRANSFORMER
	Kinds MoverKind

	// Name is the unique identifier for this resource within the plugin
	Name string

	// Spec defines the configuration parameters this resource accepts
	Spec []*Spec

	// ProvideProducer creates a new Producer instance if this resource supports PRODUCER
	ProvideProducer ProviderProducer

	// ProvideConsumer creates a new Consumer instance if this resource supports CONSUMER
	ProvideConsumer ProviderConsumer

	// ProvideTransformer creates a new Transformer instance if this resource supports TRANSFORMER
	ProvideTransformer ProviderTransformer

	// specMap is a cached map for faster spec lookups by name
	specMap map[string]*Spec
}

// Plugin represents a complete plugin with all its resources and metadata.
// This is the main structure that plugin implementations create and pass to
// RunAsClientProcess to start the gRPC server.
type Plugin struct {
	// Name is the human-readable name of this plugin
	Name string

	// Resources is the list of all data processing capabilities this plugin provides
	Resources []*Resource

	// Variables are plugin-wide variables available in HCL configurations
	Variables map[string]cty.Value

	// Functions are custom HCL functions this plugin provides
	Functions map[string]function.Function
}

// genFunc generates an HCL function for creating resource instances.
// This enables declarative resource configuration in HCL files.
func genFunc(res *Resource) function.Function {
	params := make([]function.Parameter, len(res.Spec))
	for i, spec := range res.Spec {
		params[i] = function.Parameter{
			Name:             spec.Name,
			Description:      spec.Description,
			Type:             spec.Type,
			AllowNull:        false,
			AllowUnknown:     false,
			AllowDynamicType: false,
			AllowMarked:      false,
		}
	}

	return function.New(&function.Spec{
		Description: "Generate a " + res.Name + " resource",
		Params:      params,
		Type: func(args []cty.Value) (cty.Type, error) {
			options := make(map[string]cty.Type, len(args))
			for i := range len(args) {
				options[params[i].Name] = params[i].Type
			}

			return cty.Object(map[string]cty.Type{
				"resource": cty.String,
				"options":  cty.Object(options),
			}), nil
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			options := make(map[string]cty.Value, len(args))
			for i, arg := range args {
				options[params[i].Name] = arg
			}

			return cty.ObjectVal(map[string]cty.Value{
				"resource": cty.StringVal(res.Name),
				"options":  cty.ObjectVal(options),
			}), nil
		},
		// TODO RefineResult based on spec defaults / requireds?
	})
}

// SpecMap returns a map of specification names to their definitions for fast lookup.
// The map is cached after the first call for efficiency.
func (p *Resource) SpecMap() map[string]*Spec {
	if p.specMap == nil {
		p.specMap = make(map[string]*Spec, len(p.Spec))
		for _, spec := range p.Spec {
			p.specMap[spec.Name] = spec
		}
	}

	return p.specMap
}

// Ctx returns an HCL evaluation context containing functions for all plugin resources.
// This enables HCL configurations to reference and create plugin resources declaratively.
func (p *Plugin) Ctx() *hcl.EvalContext {
	functions := make(map[string]function.Function, len(p.Resources))
	for _, res := range p.Resources {
		functions[res.Name] = genFunc(res)
	}

	return &hcl.EvalContext{
		Variables: map[string]cty.Value{},
		Functions: functions,
	}
}

// Parser is a function type for parsing configuration data into Go structs.
// Plugin implementations receive this function to parse their configuration
// parameters from the raw configuration data provided by the host.
type Parser func(interface{}) error

// Provider interfaces define how plugins create runtime instances of their capabilities.
// These interfaces are designed to be simple and avoid channels or function types
// so they can be easily adapted for both local in-process usage and remote gRPC calls.

// ProviderProducer creates Producer instances that generate data messages.
// Implementations parse configuration and return a Producer ready to start generating data.
type ProviderProducer interface {
	// ProvideProducer creates a new Producer instance using the provided configuration parser.
	// The parser function can be called with a pointer to a configuration struct to
	// populate it with the resource's configuration values.
	ProvideProducer(parse Parser) (Producer, error)
}

// ProviderConsumer creates Consumer instances that process incoming data messages.
// Implementations parse configuration and return a Consumer ready to start processing data.
type ProviderConsumer interface {
	// ProvideConsumer creates a new Consumer instance using the provided configuration parser.
	// The parser function can be called with a pointer to a configuration struct to
	// populate it with the resource's configuration values.
	ProvideConsumer(parse Parser) (Consumer, error)
}

// ProviderTransformer creates Transformer instances that transform data messages.
// Implementations parse configuration and return a Transformer ready to process messages.
type ProviderTransformer interface {
	// ProvideTransformer creates a new Transformer instance using the provided configuration parser.
	// The parser function can be called with a pointer to a configuration struct to
	// populate it with the resource's configuration values.
	ProvideTransformer(parse Parser) (Transformer, error)
}

// Runtime interfaces define the actual data processing capabilities.
// These interfaces use context and simple byte slices so they can be implemented
// locally or proxied over gRPC streams efficiently.

// Producer generates data messages and sends them to a callback function.
// Producers run until the context is cancelled or they complete their work.
type Producer interface {
	// Start begins producing data messages. The send callback should be called
	// for each message produced. Implementations must respect context cancellation
	// and return when ctx.Done() is signaled.
	//
	// The send function may return an error if the message cannot be delivered,
	// in which case the Producer should typically stop and return the error.
	Start(ctx context.Context, send func([]byte) error) error

	// Stop attempts to gracefully stop the producer. Implementations may
	// choose to implement this as a no-op if they rely solely on context cancellation.
	Stop() error
}

// Consumer processes incoming data messages received via a callback function.
// Consumers run until the context is cancelled or the message stream ends.
type Consumer interface {
	// Consume starts processing incoming messages. The recv callback should be called
	// to receive the next message. When recv returns io.EOF, it indicates the end
	// of the message stream. Implementations must respect context cancellation.
	//
	// The recv function may return other errors for delivery failures, which
	// the Consumer should handle appropriately (retry, skip, or abort).
	Consume(ctx context.Context, recv func() ([]byte, error)) error

	// Stop attempts to gracefully stop the consumer. Implementations may
	// choose to implement this as a no-op if they rely solely on context cancellation.
	Stop() error
}

// Transformer processes individual messages synchronously, transforming input to output.
// Transformers are stateless and thread-safe, processing one message at a time.
type Transformer interface {
	// Transform processes a single input message and returns the transformed result.
	// This method should be stateless and thread-safe as it may be called concurrently.
	// Return an error if the transformation fails; the original message will be
	// preserved and the error will be propagated to the caller.
	Transform(in []byte) ([]byte, error)
}
