package rpc

import (
	"context"
	"errors"
	"fmt"
	"io"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/psyduck-etl/sdk"
	"github.com/psyduck-etl/sdk/proto"
)

// client implements sdk.Plugin over a Driver stub. The schema is fetched
// once at dispense time — Name and Resources cannot return errors, so a
// plugin whose schema is unreachable fails at Dial instead.
type client struct {
	driver    proto.DriverClient
	name      string
	resources []sdk.ResourceDescriptor
}

func newClient(ctx context.Context, driver proto.DriverClient) (*client, error) {
	schema, err := driver.Schema(ctx, &proto.Empty{})
	if err != nil {
		return nil, fmt.Errorf("fetch plugin schema: %w", err)
	}
	resources, err := descriptorsFromProto(schema.Resources)
	if err != nil {
		return nil, fmt.Errorf("plugin %s: %w", schema.Name, err)
	}
	return &client{driver: driver, name: schema.Name, resources: resources}, nil
}

func (c *client) Name() string { return c.name }

func (c *client) Resources() []sdk.ResourceDescriptor { return c.resources }

func (c *client) Bind(kind sdk.Kind, resource string, block sdk.ConfigBlock) (sdk.Instance, error) {
	config, err := block.Encode()
	if err != nil {
		return nil, fmt.Errorf("plugin %q resource %q: encode config: %w", c.name, resource, err)
	}

	resp, err := c.driver.Bind(context.Background(), &proto.BindRequest{
		Kind:     uint64(kind),
		Resource: resource,
		Config:   config,
		Origin:   rangeToProto(block.Origin()),
	})
	if err != nil {
		return nil, fmt.Errorf("plugin %q resource %q: %w", c.name, resource, grpcErr(err))
	}
	return &instance{driver: c.driver, handle: resp.Instance, kind: kind, resource: resource}, nil
}

// instance is a remote handle on a bound resource living in the plugin
// subprocess. Its methods adapt the sdk's channel surface onto the Driver
// streams; the semantics documented on sdk.Producer/Consumer/Transformer
// hold across the process boundary.
type instance struct {
	driver   proto.DriverClient
	handle   uint64
	kind     sdk.Kind
	resource string
}

func (i *instance) Kind() sdk.Kind { return i.kind }

// Produce relays the remote producer's stream: data events onto send,
// error events onto errs. Mirroring well-formed producers, send is always
// closed on the way out; a transport failure is reported on errs first.
// Cancelling ctx cancels the stream, which the remote producer observes as
// its own ctx.Done.
func (i *instance) Produce(ctx context.Context, send chan<- []byte, errs chan<- error) {
	if i.kind != sdk.PRODUCER {
		panic(fmt.Sprintf("sdk/rpc: resource %q bound as kind %d, Produce called", i.resource, i.kind))
	}
	defer close(send)

	stream, err := i.driver.Produce(ctx, &proto.ProduceRequest{Instance: i.handle})
	if err != nil {
		report(ctx, errs, grpcErr(err))
		return
	}

	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			return
		}
		if err != nil {
			// Host-side cancellation surfaces here as a status error;
			// producers report ctx.Err() in that spot, so match them.
			if ctx.Err() != nil {
				report(ctx, errs, ctx.Err())
			} else {
				report(ctx, errs, grpcErr(err))
			}
			return
		}

		switch e := ev.Event.(type) {
		case *proto.Event_Data:
			select {
			case send <- e.Data:
			case <-ctx.Done():
				report(ctx, errs, ctx.Err())
				return
			}
		case *proto.Event_Error:
			report(ctx, errs, errors.New(e.Error))
		default:
			report(ctx, errs, fmt.Errorf("produce: unexpected event %T", ev.Event))
			return
		}
	}
}

// Consume relays recv onto the remote consumer and its errs/done signals
// back. The stream's send side half-closes when recv closes; a done event
// closes errs and signals done exactly like a local consumer's
// normal-completion path, while a stream that ends without one returns
// without signaling — preserving the sdk's done semantics.
func (i *instance) Consume(ctx context.Context, recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
	if i.kind != sdk.CONSUMER {
		panic(fmt.Sprintf("sdk/rpc: resource %q bound as kind %d, Consume called", i.resource, i.kind))
	}

	stream, err := i.driver.Consume(ctx)
	if err != nil {
		report(ctx, errs, grpcErr(err))
		return
	}
	if err := stream.Send(&proto.ConsumeChunk{Chunk: &proto.ConsumeChunk_Instance{Instance: i.handle}}); err != nil {
		report(ctx, errs, grpcErr(err))
		return
	}

	// Writer: recv -> stream. A Send failure means the remote side is
	// gone or finished early; keep draining recv so the host's fan-out
	// never blocks on a finished consumer, exactly like a local consumer
	// that stops processing but keeps reading. The goroutine always winds
	// down: the host closes recv (or cancels ctx) at pipeline teardown.
	go func() {
		for {
			select {
			case b, ok := <-recv:
				if !ok {
					stream.CloseSend() //nolint:errcheck // remote teardown wins
					return
				}
				if err := stream.Send(&proto.ConsumeChunk{Chunk: &proto.ConsumeChunk_Data{Data: b}}); err != nil {
					for range recv { //nolint:revive // drain until host closes recv
					}
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	doneSeen := false
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			if doneSeen {
				close(errs)
				select {
				case done <- struct{}{}:
				case <-ctx.Done():
				}
			}
			return
		}
		if err != nil {
			// Report before anything else: the host reacting to this
			// error (cancelling the pipeline) is what unblocks teardown.
			if ctx.Err() != nil {
				report(ctx, errs, ctx.Err())
			} else {
				report(ctx, errs, grpcErr(err))
			}
			return
		}

		switch e := ev.Event.(type) {
		case *proto.Event_Error:
			report(ctx, errs, errors.New(e.Error))
		case *proto.Event_Done:
			doneSeen = true
		default:
			report(ctx, errs, fmt.Errorf("consume: unexpected event %T", ev.Event))
		}
	}
}

func (i *instance) Transform(in []byte) ([]byte, error) {
	if i.kind != sdk.TRANSFORMER {
		panic(fmt.Sprintf("sdk/rpc: resource %q bound as kind %d, Transform called", i.resource, i.kind))
	}

	resp, err := i.driver.Transform(context.Background(), &proto.TransformRequest{Instance: i.handle, Data: in})
	if err != nil {
		return nil, grpcErr(err)
	}
	if resp.Error != "" {
		return nil, errors.New(resp.Error)
	}
	if resp.Drop {
		return nil, nil
	}
	if resp.Data == nil {
		// Distinguish "empty output" from the (nil, nil) drop signal.
		return []byte{}, nil
	}
	return resp.Data, nil
}

func (i *instance) Close() error {
	if _, err := i.driver.Close(context.Background(), &proto.CloseRequest{Instance: i.handle}); err != nil {
		return grpcErr(err)
	}
	return nil
}

// report forwards an error without wedging when the host has already
// abandoned the errs channel.
func report(ctx context.Context, errs chan<- error, err error) {
	select {
	case errs <- err:
	case <-ctx.Done():
	}
}

// grpcErr strips the gRPC status envelope down to its message, which for
// Bind/Transform/Close errors is the plugin-side error text.
func grpcErr(err error) error {
	if err == nil {
		return nil
	}
	if s, ok := status.FromError(err); ok && s.Code() != codes.OK {
		return errors.New(s.Message())
	}
	return err
}
