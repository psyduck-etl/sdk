package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/psyduck-etl/sdk/rpc"
)

// Example of how to use the new gRPC client system
// This would typically be in a plugin's main.go file
func main() {
	// Get socket path from environment
	socketPath := os.Getenv("PSYDUCK_SOCKET")
	if socketPath == "" {
		socketPath = "/tmp/psyduck-plugin.sock"
	}

	// Example of connecting to a running plugin
	client, err := rpc.DialUnix(socketPath)
	if err != nil {
		log.Fatalf("Failed to connect to plugin: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Test the connection
	if err := client.Ping(ctx); err != nil {
		log.Fatalf("Failed to ping plugin: %v", err)
	}

	// Get plugin information
	info, err := client.GetPluginInfo(ctx)
	if err != nil {
		log.Fatalf("Failed to get plugin info: %v", err)
	}

	log.Printf("Connected to plugin: %s", info.Plugin.Name)
	for _, resource := range info.Plugin.Resources {
		log.Printf("  Resource: %s (kinds: %v)", resource.Name, resource.Kinds)
	}

	// Example: Create a producer resource
	resourceID, err := client.CreateResource(ctx, "psyduck-constant", map[string]interface{}{
		"value":      "hello",
		"stop-after": 5,
	})
	if err != nil {
		log.Fatalf("Failed to create resource: %v", err)
	}
	log.Printf("Created resource: %s", resourceID)

	// Start producer and consume messages
	dataCh, errCh, err := client.StartProducer(ctx, resourceID)
	if err != nil {
		log.Fatalf("Failed to start producer: %v", err)
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Process messages
	go func() {
		for {
			select {
			case data, ok := <-dataCh:
				if !ok {
					log.Println("Producer finished")
					return
				}
				log.Printf("Received: %s", string(data))
			case err, ok := <-errCh:
				if !ok {
					return
				}
				log.Printf("Error: %v", err)
			case <-ctx.Done():
				return
			}
		}
	}()

	// Wait for shutdown
	<-sigChan
	log.Println("Shutting down...")
	cancel()

	// Clean up
	if err := client.StopResource(context.Background(), resourceID); err != nil {
		log.Printf("Failed to stop resource: %v", err)
	}
}