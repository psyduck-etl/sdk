# gRPC Plugin System

This SDK now uses gRPC for inter-process communication instead of the old go-plugin system.

## Key Changes

1. **Plugins run as separate processes**: Instead of exporting a `Plugin()` function, plugins now run as standalone gRPC servers.

2. **New main function pattern**: Plugins should call `sdk.RunAsClientProcess(plugin)` from their main function.

3. **Context-based interfaces**: All Producer/Consumer/Transformer interfaces now use context and callbacks instead of channels.

## Plugin Structure

### Before (old go-plugin system):
```go
func Plugin() *sdk.Plugin {
    return &sdk.Plugin{...}
}
```

### After (new gRPC system):
```go
func main() {
    plugin := &sdk.Plugin{...}
    sdk.RunAsClientProcess(plugin)
}
```

## Interface Changes

### Producer Interface
```go
type Producer interface {
    Start(ctx context.Context, send func([]byte) error) error
    Stop() error
}
```

### Consumer Interface  
```go
type Consumer interface {
    Consume(ctx context.Context, recv func() ([]byte, error)) error
    Stop() error
}
```

### Transformer Interface
```go
type Transformer interface {
    Transform(in []byte) ([]byte, error)
}
```

## Running Plugins

1. **Build the plugin**: `go build -o psyduck-plugin .`
2. **Run as gRPC server**: `PSYDUCK_SOCKET=/tmp/psyduck.sock ./psyduck-plugin`
3. **Connect from host**: Use `rpc.DialUnix("/tmp/psyduck.sock")` to connect

## Protocol Buffers

The gRPC interface is defined in `rpc/plugin.proto` with full message passing support including:
- Resource discovery
- Resource lifecycle management  
- Message streaming for producers
- Message processing for transformers
- Bidirectional communication

## Example Usage

See `example/client_example.go` for how to connect to and use a running plugin process.

## Unix Socket Communication

For now, plugins communicate via Unix sockets for efficiency and security. The socket path can be configured via the `PSYDUCK_SOCKET` environment variable.

This architecture enables:
- Better isolation between host and plugins
- Language-agnostic plugin development
- Network-based plugin deployment (future)
- Improved error handling and recovery