# API Documentation

## Package: github.com/psyduck-etl/sdk

### Core Types

#### Plugin
```go
type Plugin struct {
    Name      string                        // Plugin identifier
    Resources []*Resource                   // Available capabilities  
    Variables map[string]cty.Value          // HCL variables
    Functions map[string]function.Function  // HCL functions
}
```

Represents a complete plugin with all its resources and metadata. Pass to `RunAsClientProcess()` to start as gRPC server.

#### Resource
```go
type Resource struct {
    Kinds              MoverKind           // Capability flags (PRODUCER|CONSUMER|TRANSFORMER)
    Name               string              // Unique resource identifier
    Spec               []*Spec             // Configuration parameters
    ProvideProducer    ProviderProducer    // Creates Producer instances
    ProvideConsumer    ProviderConsumer    // Creates Consumer instances  
    ProvideTransformer ProviderTransformer // Creates Transformer instances
}
```

Defines a single data processing capability with its configuration and factory methods.

#### Spec
```go
type Spec struct {
    Name        string     // Parameter name
    Description string     // Human-readable documentation  
    Required    bool       // Whether parameter is mandatory
    Type        cty.Type   // Expected value type
    Default     cty.Value  // Default value if not required
}
```

Describes a configuration parameter for resources.

### Runtime Interfaces

#### Producer
```go
type Producer interface {
    Start(ctx context.Context, send func([]byte) error) error
    Stop() error
}
```

Generates data messages. Call `send()` for each message, respect context cancellation.

**Implementation Pattern:**
```go
type myProducer struct { config *Config }

func (p *myProducer) Start(ctx context.Context, send func([]byte) error) error {
    for i := 0; i < p.config.Count; i++ {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            if err := send([]byte(fmt.Sprintf("msg %d", i))); err != nil {
                return err
            }
        }
    }
    return nil
}

func (p *myProducer) Stop() error { return nil }
```

#### Consumer  
```go
type Consumer interface {
    Consume(ctx context.Context, recv func() ([]byte, error)) error
    Stop() error
}
```

Processes incoming messages. Call `recv()` to get next message, handle `io.EOF` for stream end.

**Implementation Pattern:**
```go
type myConsumer struct { config *Config }

func (c *myConsumer) Consume(ctx context.Context, recv func() ([]byte, error)) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            data, err := recv()
            if err == io.EOF {
                return nil
            }
            if err != nil {
                return err
            }
            // Process data
            fmt.Println(string(data))
        }
    }
}

func (c *myConsumer) Stop() error { return nil }
```

#### Transformer
```go
type Transformer interface {
    Transform(in []byte) ([]byte, error)
}
```

Transforms individual messages synchronously. Must be stateless and thread-safe.

**Implementation Pattern:**
```go
type myTransformer struct { prefix string }

func (t *myTransformer) Transform(in []byte) ([]byte, error) {
    return append([]byte(t.prefix), in...), nil
}
```

### Provider Interfaces

#### ProviderProducer
```go
type ProviderProducer interface {
    ProvideProducer(parse Parser) (Producer, error)
}
```

Factory for creating Producer instances from configuration.

**Implementation Pattern:**
```go
type myProducerProvider struct{}

func (myProducerProvider) ProvideProducer(parse Parser) (Producer, error) {
    config := new(MyConfig)
    if err := parse(config); err != nil {
        return nil, err
    }
    return &myProducer{config: config}, nil
}

var MyProducer = myProducerProvider{}
```

#### ProviderConsumer
```go
type ProviderConsumer interface {
    ProvideConsumer(parse Parser) (Consumer, error)
}
```

Factory for creating Consumer instances from configuration.

#### ProviderTransformer
```go
type ProviderTransformer interface {
    ProvideTransformer(parse Parser) (Transformer, error)
}
```

Factory for creating Transformer instances from configuration.

### Client Functions

#### RunAsClientProcess
```go
func RunAsClientProcess(plugin *Plugin)
```

Starts the plugin as a gRPC server process. Handles signals for graceful shutdown. This is the main entry point for plugin binaries.

**Usage:**
```go
func main() {
    plugin := &Plugin{...}
    RunAsClientProcess(plugin)
}
```

#### RunAsClientProcessWithContext  
```go
func RunAsClientProcessWithContext(plugin *Plugin, ctx context.Context) error
```

Starts the plugin with programmatic shutdown control via context cancellation.

### Configuration

#### Environment Variables
- `PSYDUCK_SOCKET`: Unix socket path (default: `/tmp/psyduck-plugin.sock`)

#### Parser Function
```go
type Parser func(interface{}) error
```

Parses configuration into Go structs. Use with struct tags:

```go
type Config struct {
    Host     string `psy:"host"`
    Port     int    `psy:"port"`
    Username string `psy:"username"`
}
```

## Package: github.com/psyduck-etl/sdk/rpc

### Client Types

#### GRPCClient
```go
type GRPCClient struct { /* private fields */ }
```

Client for connecting to plugin gRPC servers.

**Methods:**
```go
func DialUnix(path string) (*GRPCClient, error)
func (c *GRPCClient) Close() error
func (c *GRPCClient) Ping(ctx context.Context) error
func (c *GRPCClient) GetPluginInfo(ctx context.Context) (*pb.GetPluginInfoResponse, error)
func (c *GRPCClient) CreateResource(ctx context.Context, resourceName string, options any) (string, error)
func (c *GRPCClient) StopResource(ctx context.Context, resourceID string) error
func (c *GRPCClient) StartProducer(ctx context.Context, resourceID string) (<-chan []byte, <-chan error, error)
func (c *GRPCClient) StartConsumer(ctx context.Context, resourceID string) error
func (c *GRPCClient) SendToConsumer(ctx context.Context, data []byte) error
func (c *GRPCClient) Transform(ctx context.Context, resourceID string, data []byte) ([]byte, error)
```

**Usage Example:**
```go
client, err := rpc.DialUnix("/tmp/plugin.sock")
if err != nil {
    return err
}
defer client.Close()

// Create resource
resourceID, err := client.CreateResource(ctx, "my-producer", map[string]interface{}{
    "count": 10,
})

// Start producer
dataCh, errCh, err := client.StartProducer(ctx, resourceID)
for data := range dataCh {
    fmt.Printf("Received: %s\n", data)
}
```

### Server Types

#### GRPCServer
```go
type GRPCServer struct { /* private fields */ }
```

Server implementation for hosting plugin capabilities.

**Methods:**
```go
func NewGRPCServer(host Host) *GRPCServer
func (s *GRPCServer) StartUnix(path string) error
func (s *GRPCServer) Shutdown()
```

**Note:** Typically managed automatically by `RunAsClientProcess()`.

### Protocol Buffer Messages

The gRPC protocol is defined in `rpc/plugin.proto` with these key message types:

#### Plugin Management
- `GetPluginInfoResponse`: Plugin metadata and available resources
- `CreateResourceRequest/Response`: Resource instance creation
- `StopRequest/Response`: Resource cleanup

#### Data Messages  
- `Message`: Contains data payload, metadata, and timestamp
- `ProducerStartRequest`: Starts a producer stream
- `ConsumerStartRequest`: Initializes a consumer
- `TransformerProcessRequest/Response`: Single message transformation

#### Service Interfaces
- `PluginService`: Plugin lifecycle and resource management
- `ProducerService`: Data generation streaming
- `ConsumerService`: Data consumption handling  
- `TransformerService`: Message transformation processing

### Error Handling

gRPC errors are returned as standard Go errors. Common patterns:

```go
// Connection errors
client, err := rpc.DialUnix(path)
if err != nil {
    // Handle connection failure
}

// Resource creation errors  
resourceID, err := client.CreateResource(ctx, name, options)
if err != nil {
    // Handle creation failure
}

// Stream errors
dataCh, errCh, err := client.StartProducer(ctx, resourceID)
if err != nil {
    // Handle stream setup failure
}

select {
case data := <-dataCh:
    // Process data
case err := <-errCh:
    // Handle stream error
}
```

### Best Practices

1. **Resource Lifecycle**: Always call `StopResource()` for cleanup
2. **Context Handling**: Respect context cancellation in all operations
3. **Error Propagation**: Check errors from `send()` and `recv()` callbacks
4. **Stateless Transformers**: Keep transformers thread-safe and stateless
5. **Graceful Shutdown**: Implement proper `Stop()` methods even if no-op
6. **Socket Management**: Use unique socket paths for multiple plugins

### Performance Considerations

- **Unix Sockets**: Very efficient for local IPC
- **Resource Creation**: More expensive than usage, create once and reuse
- **Streaming**: Use channels efficiently, avoid blocking
- **Batching**: Consider batching for high-throughput scenarios
- **Memory**: Be mindful of message sizes and buffering