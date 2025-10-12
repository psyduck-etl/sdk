# Migration Guide: go-plugin to gRPC

This guide walks you through migrating your existing psyduck-etl plugins from the go-plugin architecture to the new gRPC-based system.

## Overview of Changes

The migration involves three main changes:
1. **Plugin Entry Point**: From `Plugin()` function to `main()` function
2. **Interface Updates**: From channel-based to context-based interfaces
3. **Communication**: From in-process to inter-process via gRPC

## Step 1: Update Plugin Entry Point

### Before (go-plugin)
```go
package main

import "github.com/psyduck-etl/sdk"

func Plugin() *sdk.Plugin {
    return &sdk.Plugin{
        Name: "my-plugin",
        Resources: []*sdk.Resource{
            // ... resources
        },
    }
}
```

### After (gRPC)
```go
package main

import "github.com/psyduck-etl/sdk"

func main() {
    plugin := &sdk.Plugin{
        Name: "my-plugin",
        Resources: []*sdk.Resource{
            // ... resources
        },
    }
    
    // Run as gRPC client process
    sdk.RunAsClientProcess(plugin)
}
```

## Step 2: Update Resource Spec Structure

### Before
```go
Spec: sdk.SpecMap{
    "field": &sdk.Spec{
        Name:        "field",
        Description: "Field description",
        Type:        cty.String,
        Required:    true,
    },
}
```

### After
```go
Spec: []*sdk.Spec{
    {
        Name:        "field",
        Description: "Field description",
        Type:        cty.String,
        Required:    true,
    },
}
```

## Step 3: Update Producer Implementation

### Before (channel-based)
```go
func MyProducer(parse sdk.Parser, specParse sdk.SpecParser) (sdk.Producer, error) {
    config := new(MyConfig)
    if err := parse(config); err != nil {
        return nil, err
    }
    
    return func(send chan<- []byte, errs chan<- error) {
        defer close(send)
        defer close(errs)
        
        for i := 0; i < config.Count; i++ {
            send <- []byte(fmt.Sprintf("message %d", i))
        }
    }, nil
}
```

### After (context-based)
```go
type myProducer struct {
    config *MyConfig
}

func (p *myProducer) Start(ctx context.Context, send func([]byte) error) error {
    for i := 0; i < p.config.Count; i++ {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            if err := send([]byte(fmt.Sprintf("message %d", i))); err != nil {
                return err
            }
        }
    }
    return nil
}

func (p *myProducer) Stop() error {
    return nil // Cleanup if needed
}

type myProducerProvider struct{}

func (myProducerProvider) ProvideProducer(parse sdk.Parser) (sdk.Producer, error) {
    config := new(MyConfig)
    if err := parse(config); err != nil {
        return nil, err
    }
    
    return &myProducer{config: config}, nil
}

var MyProducer = myProducerProvider{}
```

## Step 4: Update Consumer Implementation

### Before (channel-based)
```go
func MyConsumer(parse sdk.Parser, specParse sdk.SpecParser) (sdk.Consumer, error) {
    return func(recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
        defer close(done)
        defer close(errs)
        
        for data := range recv {
            // Process data
            fmt.Println(string(data))
        }
    }, nil
}
```

### After (context-based)
```go
type myConsumer struct {
    config *MyConfig
}

func (c *myConsumer) Consume(ctx context.Context, recv func() ([]byte, error)) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
            data, err := recv()
            if err == io.EOF {
                return nil // End of stream
            }
            if err != nil {
                return err
            }
            
            // Process data
            fmt.Println(string(data))
        }
    }
}

func (c *myConsumer) Stop() error {
    return nil // Cleanup if needed
}

type myConsumerProvider struct{}

func (myConsumerProvider) ProvideConsumer(parse sdk.Parser) (sdk.Consumer, error) {
    config := new(MyConfig)
    if err := parse(config); err != nil {
        return nil, err
    }
    
    return &myConsumer{config: config}, nil
}

var MyConsumer = myConsumerProvider{}
```

## Step 5: Update Transformer Implementation

### Before (function-based)
```go
func MyTransformer(parse sdk.Parser, specParse sdk.SpecParser) (sdk.Transformer, error) {
    config := new(MyConfig)
    if err := parse(config); err != nil {
        return nil, err
    }
    
    return func(data []byte) ([]byte, error) {
        // Transform data
        return append([]byte(config.Prefix), data...), nil
    }, nil
}
```

### After (interface-based)
```go
type myTransformer struct {
    config *MyConfig
}

func (t *myTransformer) Transform(data []byte) ([]byte, error) {
    // Transform data
    return append([]byte(t.config.Prefix), data...), nil
}

type myTransformerProvider struct{}

func (myTransformerProvider) ProvideTransformer(parse sdk.Parser) (sdk.Transformer, error) {
    config := new(MyConfig)
    if err := parse(config); err != nil {
        return nil, err
    }
    
    return &myTransformer{config: config}, nil
}

var MyTransformer = myTransformerProvider{}
```

## Step 6: Update Resource Definitions

### Before
```go
{
    Name:               "my-transformer",
    Kinds:              sdk.TRANSFORMER,
    ProvideTransformer: MyTransformer,
    Spec: sdk.SpecMap{
        "prefix": &sdk.Spec{
            Name:        "prefix",
            Description: "Prefix to add",
            Type:        cty.String,
            Required:    true,
        },
    },
}
```

### After
```go
{
    Name:               "my-transformer",
    Kinds:              sdk.TRANSFORMER,
    ProvideTransformer: MyTransformer,
    Spec: []*sdk.Spec{
        {
            Name:        "prefix",
            Description: "Prefix to add",
            Type:        cty.String,
            Required:    true,
        },
    },
}
```

## Step 7: Update go.mod

Add a replace directive to use the local SDK:

```go
module github.com/your-org/your-plugin

go 1.22.1

require (
    github.com/psyduck-etl/sdk v0.3.0
    github.com/zclconf/go-cty v1.14.4
)

// Use local SDK during development
replace github.com/psyduck-etl/sdk => ../sdk
```

## Step 8: Build and Run

### Build the plugin
```bash
go build -o my-plugin .
```

### Run as gRPC server
```bash
PSYDUCK_SOCKET=/tmp/my-plugin.sock ./my-plugin
```

### Connect from host application
```go
client, err := rpc.DialUnix("/tmp/my-plugin.sock")
if err != nil {
    log.Fatal(err)
}
defer client.Close()

// Get plugin info
info, err := client.GetPluginInfo(ctx)
if err != nil {
    log.Fatal(err)
}

// Create and use resources
resourceID, err := client.CreateResource(ctx, "my-transformer", map[string]interface{}{
    "prefix": "transformed: ",
})
```

## Breaking Changes Summary

1. **Removed `SpecParser` parameter**: Provider functions now only take `sdk.Parser`
2. **Removed `SpecMap` type**: Use `[]*sdk.Spec` slice instead
3. **Channel-based interfaces replaced**: All interfaces now use context and callbacks
4. **Provider pattern required**: Producers, consumers, and transformers must implement provider interfaces
5. **Entry point changed**: `Plugin()` function replaced with `main()` + `sdk.RunAsClientProcess()`

## Common Pitfalls

1. **Forgetting to handle context cancellation**: Always check `ctx.Done()` in loops
2. **Not implementing Stop() methods**: Even if no cleanup is needed, implement as no-op
3. **Using old SpecMap syntax**: Switch to slice of Spec pointers
4. **Missing provider pattern**: Don't return function literals, implement proper interfaces

## Testing Your Migration

1. Verify the plugin builds without errors
2. Test starting the plugin as a server process
3. Test connecting with the gRPC client
4. Verify all resources can be created and used
5. Test graceful shutdown with signals

## Performance Considerations

- gRPC adds some overhead compared to in-process calls
- Unix sockets are very efficient for local communication
- Consider batching for high-throughput scenarios
- Resource creation is now more expensive (separate from usage)

## Debugging Tips

1. **Enable gRPC logging**: Set `GRPC_GO_LOG_VERBOSITY_LEVEL=99` and `GRPC_GO_LOG_SEVERITY_LEVEL=info`
2. **Check socket permissions**: Ensure the socket file is accessible
3. **Monitor resource lifecycle**: Use the Ping method to verify connectivity
4. **Test context cancellation**: Verify proper cleanup on shutdown

## Future Migration Path

This gRPC architecture enables:
- **Network deployment**: Plugins can run on different machines
- **Language agnostic plugins**: Write plugins in any language with gRPC support  
- **Better isolation**: Plugins run in separate processes
- **Improved monitoring**: gRPC provides built-in metrics and tracing