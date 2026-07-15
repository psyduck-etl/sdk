package rpc

import (
	"context"
	"errors"
	"io"
	"sync"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/psyduck-etl/sdk"
	"github.com/psyduck-etl/sdk/proto"
)

// driverServer serves one sdk.Plugin to the host. Bound instances live in
// a handle table; every instance RPC resolves its handle through it.
type driverServer struct {
	proto.UnimplementedDriverServer

	impl sdk.Plugin

	mu        sync.Mutex
	next      uint64
	instances map[uint64]sdk.Instance
}

func newServer(impl sdk.Plugin) *driverServer {
	return &driverServer{impl: impl, instances: make(map[uint64]sdk.Instance)}
}

func (s *driverServer) lookup(handle uint64) (sdk.Instance, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inst, ok := s.instances[handle]
	if !ok {
		return nil, status.Errorf(codes.NotFound, "no instance %d — was it bound (and not closed) on this connection?", handle)
	}
	return inst, nil
}

func (s *driverServer) Schema(ctx context.Context, _ *proto.Empty) (*proto.SchemaResponse, error) {
	resources, err := descriptorsToProto(s.impl.Resources())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.SchemaResponse{Name: s.impl.Name(), Resources: resources}, nil
}

func (s *driverServer) Bind(ctx context.Context, req *proto.BindRequest) (*proto.BindResponse, error) {
	block := sdk.NewJSONBlock(rangeFromProto(req.Origin), req.Config)
	inst, err := s.impl.Bind(sdk.Kind(req.Kind), req.Resource, block)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	s.mu.Lock()
	s.next++
	handle := s.next
	s.instances[handle] = inst
	s.mu.Unlock()

	return &proto.BindResponse{Instance: handle}, nil
}

// Produce runs the instance's producer, forwarding its send/errs channels
// onto the stream as batch/error events. The stream ends cleanly when the
// producer has both closed send and returned; host-side cancellation
// arrives as stream-context cancellation, which the producer sees as
// ctx.Done.
func (s *driverServer) Produce(req *proto.ProduceRequest, stream proto.Driver_ProduceServer) error {
	inst, err := s.lookup(req.Instance)
	if err != nil {
		return err
	}

	ctx := stream.Context()
	send := make(chan []byte, batchItems)
	errs := make(chan error)
	returned := make(chan struct{})

	go func() {
		defer close(returned)
		inst.Produce(ctx, send, errs)
	}()

	return muxEvents(ctx, stream, send, errs, returned, nil)
}

// muxEvents forwards a resource's data/errs channels onto stream as
// batch/error events until the resource is finished. The sdk contracts
// make data's close the completion signal, but a resource that returns
// without closing its output channel (a bug) must not wedge the host, so
// the mux also ends when the resource function returns. Resources that
// close errs on the way out are tolerated: a closed channel is simply
// dropped from the select. readErr, when non-nil, aborts the mux with a
// stream-reader failure (nil channels never fire).
func muxEvents(ctx context.Context, stream interface {
	Send(*proto.Event) error
}, data <-chan []byte, errs <-chan error, returned <-chan struct{}, readErr <-chan error) error {
	pending := returned
	for data != nil {
		select {
		case b, ok := <-data:
			if !ok {
				data = nil
				continue
			}
			batch := &proto.Batch{Items: gather(b, data)}
			if err := stream.Send(&proto.Event{Event: &proto.Event_Batch{Batch: batch}}); err != nil {
				return err
			}
		case e, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err := sendError(stream, e); err != nil {
				return err
			}
		case <-pending:
			pending = nil // resource returned; stop selecting on it
			// The resource's channels can gain nothing more, but data is
			// buffered and may still hold items sent before the return —
			// drain those, then stop selecting on data so a resource that
			// returned without closing it (a bug) cannot wedge the host.
			for data != nil {
				select {
				case b, ok := <-data:
					if !ok {
						data = nil
						continue
					}
					batch := &proto.Batch{Items: gather(b, data)}
					if err := stream.Send(&proto.Event{Event: &proto.Event_Batch{Batch: batch}}); err != nil {
						return err
					}
				default:
					data = nil
				}
			}
		case err := <-readErr:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// data is closed; the resource may still be running and reporting
	// errors (errs is sent to strictly before the resource returns, so
	// waiting on returned cannot miss one).
	for {
		select {
		case e, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err := sendError(stream, e); err != nil {
				return err
			}
		case <-returned:
			return nil
		case err := <-readErr:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Consume feeds inbound chunks to the instance's consumer and relays its
// errs/done signals back as events. The host half-closing its send side is
// the consumer's recv close; the consumer's done signal crosses as a done
// event followed by a clean stream end. A consumer that returns without
// signaling done ends the stream with no done event — the host side
// preserves that distinction.
func (s *driverServer) Consume(stream proto.Driver_ConsumeServer) error {
	first, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "consume: reading instance handle: %v", err)
	}
	handle, ok := first.Chunk.(*proto.DataChunk_Instance)
	if !ok {
		return status.Error(codes.InvalidArgument, "consume: first chunk must carry the instance handle")
	}
	inst, err := s.lookup(handle.Instance)
	if err != nil {
		return err
	}

	ctx := stream.Context()
	recv := make(chan []byte)
	errs := make(chan error)
	done := make(chan struct{})
	returned := make(chan struct{})

	go func() {
		defer close(returned)
		inst.Consume(ctx, recv, errs, done)
	}()

	// Reader: host chunks -> recv, closing recv when the host half-closes.
	// Consumer teardown (returned) unblocks a stranded recv send.
	readErr := make(chan error, 1)
	go func() {
		defer close(recv)
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				readErr <- err
				return
			}
			batch, ok := chunk.Chunk.(*proto.DataChunk_Batch)
			if !ok {
				readErr <- status.Error(codes.InvalidArgument, "consume: duplicate instance chunk")
				return
			}
			for _, item := range batch.Batch.Items {
				select {
				case recv <- item:
				case <-returned:
					return
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	for {
		select {
		case e, ok := <-errs:
			if !ok {
				// The consumer closed errs on its normal-completion path;
				// its done signal is next.
				errs = nil
				continue
			}
			if err := sendError(stream, e); err != nil {
				return err
			}
		case <-done:
			return stream.Send(&proto.Event{Event: &proto.Event_Done{Done: true}})
		case <-returned:
			// The consumer returned without signaling done (error/abort
			// path). done may still be closed-not-sent; check once more
			// so close(done) implementations aren't misread as aborts.
			select {
			case <-done:
				return stream.Send(&proto.Event{Event: &proto.Event_Done{Done: true}})
			default:
			}
			return nil
		case err := <-readErr:
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Transform runs the instance's transformer as a long-running stage:
// inbound chunks feed its in channel, and its out/errs channels flow back
// as batch/error events. The host half-closing its send side is the
// transformer's in close (its cue to flush); the transformer closing out
// ends the stream cleanly once any trailing errors have crossed.
func (s *driverServer) Transform(stream proto.Driver_TransformServer) error {
	first, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "transform: reading instance handle: %v", err)
	}
	handle, ok := first.Chunk.(*proto.DataChunk_Instance)
	if !ok {
		return status.Error(codes.InvalidArgument, "transform: first chunk must carry the instance handle")
	}
	inst, err := s.lookup(handle.Instance)
	if err != nil {
		return err
	}

	ctx := stream.Context()
	in := make(chan []byte, batchItems)
	out := make(chan []byte, batchItems)
	errs := make(chan error)
	returned := make(chan struct{})

	go func() {
		defer close(returned)
		inst.Transform(ctx, in, out, errs)
	}()

	// Reader: host chunks -> in, closing in when the host half-closes.
	// Transformer teardown (returned) unblocks a stranded in send.
	readErr := make(chan error, 1)
	go func() {
		defer close(in)
		for {
			chunk, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				readErr <- err
				return
			}
			batch, ok := chunk.Chunk.(*proto.DataChunk_Batch)
			if !ok {
				readErr <- status.Error(codes.InvalidArgument, "transform: duplicate instance chunk")
				return
			}
			for _, item := range batch.Batch.Items {
				select {
				case in <- item:
				case <-returned:
					return
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return muxEvents(ctx, stream, out, errs, returned, readErr)
}

func (s *driverServer) Close(ctx context.Context, req *proto.CloseRequest) (*proto.Empty, error) {
	s.mu.Lock()
	inst, ok := s.instances[req.Instance]
	delete(s.instances, req.Instance)
	s.mu.Unlock()

	if !ok {
		return nil, status.Errorf(codes.NotFound, "no instance %d", req.Instance)
	}
	if err := inst.Close(); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &proto.Empty{}, nil
}

// sendError relays a resource error as an error event. A nil error still
// crosses (as its rendering) so host and plugin never disagree about how
// many errors were reported.
func sendError(stream interface {
	Send(*proto.Event) error
}, e error) error {
	if e == nil {
		e = errors.New("<nil>")
	}
	return stream.Send(&proto.Event{Event: &proto.Event_Error{Error: e.Error()}})
}
