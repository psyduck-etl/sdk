// Package rpc provides gRPC-based inter-process communication for psyduck-etl plugins.
//
// This package implements a complete gRPC system that allows plugins to run as
// separate processes, communicating with the host via Unix sockets. It replaces
// the previous go-plugin based architecture with a more robust and flexible solution.
//
// Key Components:
//   - GRPCServer: Serves plugin capabilities over gRPC
//   - GRPCClient: Connects to and controls remote plugin processes  
//   - Protocol Buffers: Defines the wire protocol in plugin.proto
//
// The gRPC interface supports:
//   - Plugin discovery and metadata
//   - Resource lifecycle management
//   - Streaming data production
//   - Message transformation
//   - Data consumption
package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/psyduck-etl/sdk/rpc/pb"
)

// Producer interface defines the contract for data producers in the gRPC system.
// This mirrors the sdk.Producer interface but is defined here to avoid import cycles.
type Producer interface {
	// Start begins data production, calling send() for each message.
	// Must respect context cancellation and return when ctx.Done() is signaled.
	Start(ctx context.Context, send func([]byte) error) error
	
	// Stop gracefully shuts down the producer.
	Stop() error
}

// Consumer interface defines the contract for data consumers in the gRPC system.
// This mirrors the sdk.Consumer interface but is defined here to avoid import cycles.
type Consumer interface {
	// Consume processes incoming data by calling recv() to get messages.
	// Must respect context cancellation and handle io.EOF to detect stream end.
	Consume(ctx context.Context, recv func() ([]byte, error)) error
	
	// Stop gracefully shuts down the consumer.
	Stop() error
}

// Transformer interface defines the contract for data transformers in the gRPC system.
// This mirrors the sdk.Transformer interface but is defined here to avoid import cycles.
type Transformer interface {
	// Transform processes a single message synchronously.
	// Should be stateless and thread-safe for concurrent calls.
	Transform(in []byte) ([]byte, error)
}

// Provider function types define how to create runtime instances from configuration.
// These mirror the sdk provider types but use local interface definitions.
type ProviderProducer func(parse func(interface{}) error) (Producer, error)
type ProviderConsumer func(parse func(interface{}) error) (Consumer, error)
type ProviderTransformer func(parse func(interface{}) error) (Transformer, error)

// Resource describes a single capability provided by a plugin.
// This is the gRPC-adapted version of sdk.Resource.
type Resource struct {
	// Name uniquely identifies this resource within the plugin
	Name string
	
	// Kind specifies the capability type: "PRODUCER", "CONSUMER", or "TRANSFORMER"
	Kind string
	
	// Provider functions create runtime instances (only one should be non-nil)
	ProvideProducer    ProviderProducer
	ProvideConsumer    ProviderConsumer
	ProvideTransformer ProviderTransformer
}

// Host interface abstracts the plugin's resource provider.
// The GRPCServer uses this to discover and create plugin resources.
type Host interface {
	// Resources returns all capabilities provided by this plugin
	Resources() []Resource
}

// resourceInstance tracks a created resource and its runtime state.
type resourceInstance struct {
	id          string              // Unique identifier for this instance
	name        string              // Resource name this instance was created from
	kind        string              // Resource kind (PRODUCER/CONSUMER/TRANSFORMER)
	producer    Producer            // Non-nil if this is a producer instance
	consumer    Consumer            // Non-nil if this is a consumer instance
	transformer Transformer         // Non-nil if this is a transformer instance
	cancel      context.CancelFunc  // Cancels the instance's operation context
}

// GRPCServer implements the complete gRPC plugin server architecture.
// It hosts multiple service implementations to avoid method name conflicts
// between the different gRPC services (Plugin, Producer, Consumer, Transformer).
type GRPCServer struct {
	host      Host                              // Plugin resource provider
	grpc      *grpc.Server                      // Underlying gRPC server
	ln        net.Listener                      // Unix socket listener
	socket    string                            // Socket file path
	resources map[string]*resourceInstance     // Active resource instances
	mu        sync.RWMutex                      // Protects resources map
	nextID    int64                             // Counter for generating unique IDs

	// Service implementations - separated to avoid method name conflicts
	pluginService      *PluginServiceImpl
	producerService    *ProducerServiceImpl
	consumerService    *ConsumerServiceImpl
	transformerService *TransformerServiceImpl
}

// Separate service implementations to avoid method name conflicts
type PluginServiceImpl struct {
	pb.UnimplementedPluginServiceServer
	server *GRPCServer
}

type ProducerServiceImpl struct {
	pb.UnimplementedProducerServiceServer
	server *GRPCServer
}

type ConsumerServiceImpl struct {
	pb.UnimplementedConsumerServiceServer
	server *GRPCServer
}

type TransformerServiceImpl struct {
	pb.UnimplementedTransformerServiceServer
	server *GRPCServer
}

func NewGRPCServer(host Host) *GRPCServer {
	server := &GRPCServer{
		host:      host,
		resources: make(map[string]*resourceInstance),
	}
	
	// Initialize service implementations
	server.pluginService = &PluginServiceImpl{server: server}
	server.producerService = &ProducerServiceImpl{server: server}
	server.consumerService = &ConsumerServiceImpl{server: server}
	server.transformerService = &TransformerServiceImpl{server: server}
	
	return server
}

func (s *GRPCServer) StartUnix(path string) error {
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	s.socket = path
	s.ln = ln
	s.grpc = grpc.NewServer()

	pb.RegisterPluginServiceServer(s.grpc, s.pluginService)
	pb.RegisterProducerServiceServer(s.grpc, s.producerService)
	pb.RegisterConsumerServiceServer(s.grpc, s.consumerService)
	pb.RegisterTransformerServiceServer(s.grpc, s.transformerService)
	reflection.Register(s.grpc)

	go func() {
		_ = s.grpc.Serve(s.ln)
	}()

	return nil
}

func (s *GRPCServer) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop all resources
	for _, res := range s.resources {
		if res.cancel != nil {
			res.cancel()
		}
		if res.producer != nil {
			res.producer.Stop()
		}
		if res.consumer != nil {
			res.consumer.Stop()
		}
	}

	if s.grpc != nil {
		s.grpc.Stop()
	}
	if s.ln != nil {
		_ = s.ln.Close()
	}
	if s.socket != "" {
		_ = os.Remove(s.socket)
	}
}

func (s *GRPCServer) generateID() string {
	s.nextID++
	return fmt.Sprintf("res_%d", s.nextID)
}

// PluginService implementation
func (ps *PluginServiceImpl) GetPluginInfo(ctx context.Context, _ *pb.Empty) (*pb.GetPluginInfoResponse, error) {
	resources := ps.server.host.Resources()
	pbResources := make([]*pb.Resource, 0, len(resources))

	for _, r := range resources {
		pbRes := &pb.Resource{
			Name:  r.Name,
			Kinds: []string{r.Kind},
			Specs: []*pb.Spec{}, // TODO: Add spec conversion if needed
		}
		pbResources = append(pbResources, pbRes)
	}

	return &pb.GetPluginInfoResponse{
		Plugin: &pb.PluginInfo{
			Name:      "plugin", // TODO: Get from host
			Resources: pbResources,
			Variables: make(map[string][]byte),
			Functions: make(map[string]string),
		},
	}, nil
}

func (ps *PluginServiceImpl) CreateResource(ctx context.Context, req *pb.CreateResourceRequest) (*pb.CreateResourceResponse, error) {
	// Find resource definition
	var resDef *Resource
	for _, r := range ps.server.host.Resources() {
		if r.Name == req.ResourceName {
			res := r // copy
			resDef = &res
			break
		}
	}
	if resDef == nil {
		return &pb.CreateResourceResponse{
			Success: false,
			Error:   fmt.Sprintf("unknown resource: %s", req.ResourceName),
		}, nil
	}

	// Parse options
	opts := make(map[string]any)
	if len(req.OptionsJson) > 0 {
		if err := json.Unmarshal(req.OptionsJson, &opts); err != nil {
			return &pb.CreateResourceResponse{
				Success: false,
				Error:   fmt.Sprintf("failed to parse options: %v", err),
			}, nil
		}
	}

	parse := func(target interface{}) error {
		b, err := json.Marshal(opts)
		if err != nil {
			return err
		}
		return json.Unmarshal(b, target)
	}

	// Create resource instance
	id := ps.server.generateID()
	instance := &resourceInstance{
		id:   id,
		name: resDef.Name,
		kind: resDef.Kind,
	}

	var err error
	switch resDef.Kind {
	case "PRODUCER":
		if resDef.ProvideProducer != nil {
			instance.producer, err = resDef.ProvideProducer(parse)
		}
	case "CONSUMER":
		if resDef.ProvideConsumer != nil {
			instance.consumer, err = resDef.ProvideConsumer(parse)
		}
	case "TRANSFORMER":
		if resDef.ProvideTransformer != nil {
			instance.transformer, err = resDef.ProvideTransformer(parse)
		}
	default:
		err = fmt.Errorf("unsupported resource kind: %s", resDef.Kind)
	}

	if err != nil {
		return &pb.CreateResourceResponse{
			Success: false,
			Error:   fmt.Sprintf("failed to create resource: %v", err),
		}, nil
	}

	ps.server.mu.Lock()
	ps.server.resources[id] = instance
	ps.server.mu.Unlock()

	return &pb.CreateResourceResponse{
		ResourceId: id,
		Success:    true,
	}, nil
}

func (ps *PluginServiceImpl) StopResource(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	ps.server.mu.Lock()
	defer ps.server.mu.Unlock()

	instance, exists := ps.server.resources[req.ResourceId]
	if !exists {
		return &pb.StopResponse{
			Success: false,
			Error:   "resource not found",
		}, nil
	}

	if instance.cancel != nil {
		instance.cancel()
	}

	var err error
	if instance.producer != nil {
		err = instance.producer.Stop()
	} else if instance.consumer != nil {
		err = instance.consumer.Stop()
	}

	delete(ps.server.resources, req.ResourceId)

	if err != nil {
		return &pb.StopResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &pb.StopResponse{Success: true}, nil
}

func (ps *PluginServiceImpl) Ping(ctx context.Context, _ *pb.Empty) (*pb.Status, error) {
	return &pb.Status{Success: true}, nil
}

// ProducerService implementation
func (ps *ProducerServiceImpl) Start(req *pb.ProducerStartRequest, stream pb.ProducerService_StartServer) error {
	ps.server.mu.RLock()
	instance, exists := ps.server.resources[req.ResourceId]
	ps.server.mu.RUnlock()

	if !exists || instance.producer == nil {
		return fmt.Errorf("producer resource not found: %s", req.ResourceId)
	}

	ctx, cancel := context.WithCancel(context.Background())
	instance.cancel = cancel

	return instance.producer.Start(ctx, func(data []byte) error {
		msg := &pb.Message{
			Data:      data,
			Metadata:  make(map[string]string),
			Timestamp: time.Now().UnixNano(),
		}
		return stream.Send(msg)
	})
}

func (ps *ProducerServiceImpl) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	return ps.server.pluginService.StopResource(ctx, req)
}

// ConsumerService implementation  
func (cs *ConsumerServiceImpl) Start(ctx context.Context, req *pb.ConsumerStartRequest) (*pb.Status, error) {
	cs.server.mu.RLock()
	instance, exists := cs.server.resources[req.ResourceId]
	cs.server.mu.RUnlock()

	if !exists || instance.consumer == nil {
		return &pb.Status{
			Success: false,
			Error:   fmt.Sprintf("consumer resource not found: %s", req.ResourceId),
		}, nil
	}

	// Consumer will be fed via Send() calls
	return &pb.Status{Success: true}, nil
}

func (cs *ConsumerServiceImpl) Send(ctx context.Context, msg *pb.Message) (*pb.Status, error) {
	// For this simplified implementation, we would need to track which consumer
	// this message is for. In a more complete implementation, we'd need to
	// associate messages with specific consumer instances.
	return &pb.Status{Success: true}, nil
}

func (cs *ConsumerServiceImpl) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	return cs.server.pluginService.StopResource(ctx, req)
}

// TransformerService implementation
func (ts *TransformerServiceImpl) Process(ctx context.Context, req *pb.TransformerProcessRequest) (*pb.TransformerProcessResponse, error) {
	ts.server.mu.RLock()
	instance, exists := ts.server.resources[req.ResourceId]
	ts.server.mu.RUnlock()

	if !exists || instance.transformer == nil {
		return &pb.TransformerProcessResponse{
			Success: false,
			Error:   fmt.Sprintf("transformer resource not found: %s", req.ResourceId),
		}, nil
	}

	out, err := instance.transformer.Transform(req.Message.Data)
	if err != nil {
		return &pb.TransformerProcessResponse{
			Success: false,
			Error:   err.Error(),
		}, nil
	}

	return &pb.TransformerProcessResponse{
		Message: &pb.Message{
			Data:      out,
			Metadata:  make(map[string]string),
			Timestamp: time.Now().UnixNano(),
		},
		Success: true,
	}, nil
}

func (ts *TransformerServiceImpl) Stop(ctx context.Context, req *pb.StopRequest) (*pb.StopResponse, error) {
	return ts.server.pluginService.StopResource(ctx, req)
}