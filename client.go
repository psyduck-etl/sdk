package sdk

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/psyduck-etl/sdk/rpc"
)

// RunAsClientProcess runs the plugin as a standalone gRPC client process.
// This is the new entry point for plugins, replacing the old Plugin() function export pattern.
// 
// The function will:
// 1. Create a gRPC server listening on a Unix socket
// 2. Register all plugin resources with the server
// 3. Handle graceful shutdown on SIGINT/SIGTERM
//
// Socket path can be configured via PSYDUCK_SOCKET environment variable,
// defaults to "/tmp/psyduck-plugin.sock".
//
// Example usage in plugin main():
//
//	func main() {
//	    plugin := &sdk.Plugin{
//	        Name: "my-plugin",
//	        Resources: []*sdk.Resource{...},
//	    }
//	    sdk.RunAsClientProcess(plugin)
//	}
func RunAsClientProcess(plugin *Plugin) {
	// Get socket path from environment or use default
	socketPath := os.Getenv("PSYDUCK_SOCKET")
	if socketPath == "" {
		socketPath = "/tmp/psyduck-plugin.sock"
	}

	// Convert sdk.Plugin to rpc.Host using the adapter
	adapter := &pluginAdapter{plugin: plugin}
	server := rpc.NewGRPCServer(adapter)

	// Start the gRPC server
	if err := server.StartUnix(socketPath); err != nil {
		log.Fatalf("failed to start gRPC server: %v", err)
	}

	log.Printf("Plugin '%s' started on socket: %s", plugin.Name, socketPath)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	log.Println("Shutting down plugin...")

	server.Shutdown()
}

// RunAsClientProcessWithContext runs the plugin with a custom context for shutdown control.
// This variant allows programmatic shutdown via context cancellation instead of signal handling.
//
// Example usage:
//
//	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
//	defer cancel()
//	
//	if err := sdk.RunAsClientProcessWithContext(plugin, ctx); err != nil {
//	    log.Fatalf("Plugin failed: %v", err)
//	}
func RunAsClientProcessWithContext(plugin *Plugin, ctx context.Context) error {
	// Get socket path from environment or use default
	socketPath := os.Getenv("PSYDUCK_SOCKET")
	if socketPath == "" {
		socketPath = "/tmp/psyduck-plugin.sock"
	}

	// Convert sdk.Plugin to rpc.Host using the adapter
	adapter := &pluginAdapter{plugin: plugin}
	server := rpc.NewGRPCServer(adapter)

	// Start the gRPC server
	if err := server.StartUnix(socketPath); err != nil {
		return fmt.Errorf("failed to start gRPC server: %w", err)
	}

	log.Printf("Plugin '%s' started on socket: %s", plugin.Name, socketPath)

	// Wait for context cancellation
	<-ctx.Done()
	log.Println("Shutting down plugin...")

	server.Shutdown()
	return nil
}

// pluginAdapter adapts an sdk.Plugin to the rpc.Host interface.
// This adapter bridges the gap between the SDK's plugin model and the
// gRPC server's expected interface, handling the conversion of provider
// functions and capability mappings.
type pluginAdapter struct {
	plugin *Plugin
}

// Resources converts SDK resources to RPC resources, mapping capabilities
// and creating provider function adapters for gRPC compatibility.
func (a *pluginAdapter) Resources() []rpc.Resource {
	out := make([]rpc.Resource, 0, len(a.plugin.Resources))
	for _, r := range a.plugin.Resources {
		res := rpc.Resource{Name: r.Name}

		// Determine primary kind - currently we only support one kind per resource
		// in the gRPC interface, though the SDK supports multiple kinds
		if r.Kinds&PRODUCER != 0 {
			res.Kind = "PRODUCER"
		} else if r.Kinds&CONSUMER != 0 {
			res.Kind = "CONSUMER"
		} else if r.Kinds&TRANSFORMER != 0 {
			res.Kind = "TRANSFORMER"
		}

		// Capture loop variable to avoid closure issues
		rLocal := r
		
		// Adapt SDK provider functions to RPC provider functions
		res.ProvideProducer = func(parse func(interface{}) error) (rpc.Producer, error) {
			if rLocal.ProvideProducer == nil {
				return nil, fmt.Errorf("resource %s does not provide producer", rLocal.Name)
			}
			prod, err := rLocal.ProvideProducer.ProvideProducer(parse)
			if err != nil {
				return nil, err
			}
			return prod, nil
		}

		res.ProvideConsumer = func(parse func(interface{}) error) (rpc.Consumer, error) {
			if rLocal.ProvideConsumer == nil {
				return nil, fmt.Errorf("resource %s does not provide consumer", rLocal.Name)
			}
			cons, err := rLocal.ProvideConsumer.ProvideConsumer(parse)
			if err != nil {
				return nil, err
			}
			return cons, nil
		}

		res.ProvideTransformer = func(parse func(interface{}) error) (rpc.Transformer, error) {
			if rLocal.ProvideTransformer == nil {
				return nil, fmt.Errorf("resource %s does not provide transformer", rLocal.Name)
			}
			trans, err := rLocal.ProvideTransformer.ProvideTransformer(parse)
			if err != nil {
				return nil, err
			}
			return trans, nil
		}

		out = append(out, res)
	}
	return out
}