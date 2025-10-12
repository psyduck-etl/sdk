package rpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/psyduck-etl/sdk/rpc/pb"
)

// GRPCClient wraps a gRPC connection to a plugin process
type GRPCClient struct {
	conn        *grpc.ClientConn
	plugin      pb.PluginServiceClient
	producer    pb.ProducerServiceClient
	consumer    pb.ConsumerServiceClient
	transformer pb.TransformerServiceClient
}

func DialUnix(path string) (*GRPCClient, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, "unix://"+path,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return net.Dial("unix", path)
		}))
	if err != nil {
		return nil, err
	}

	c := &GRPCClient{
		conn:        conn,
		plugin:      pb.NewPluginServiceClient(conn),
		producer:    pb.NewProducerServiceClient(conn),
		consumer:    pb.NewConsumerServiceClient(conn),
		transformer: pb.NewTransformerServiceClient(conn),
	}

	return c, nil
}

func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

func (c *GRPCClient) Ping(ctx context.Context) error {
	_, err := c.plugin.Ping(ctx, &pb.Empty{})
	return err
}

func (c *GRPCClient) GetPluginInfo(ctx context.Context) (*pb.GetPluginInfoResponse, error) {
	return c.plugin.GetPluginInfo(ctx, &pb.Empty{})
}

func (c *GRPCClient) CreateResource(ctx context.Context, resourceName string, options any) (string, error) {
	opts := []byte(nil)
	if options != nil {
		b, err := json.Marshal(options)
		if err != nil {
			return "", err
		}
		opts = b
	}

	resp, err := c.plugin.CreateResource(ctx, &pb.CreateResourceRequest{
		ResourceName: resourceName,
		OptionsJson:  opts,
	})
	if err != nil {
		return "", err
	}

	if !resp.Success {
		return "", fmt.Errorf("failed to create resource: %s", resp.Error)
	}

	return resp.ResourceId, nil
}

func (c *GRPCClient) StopResource(ctx context.Context, resourceID string) error {
	resp, err := c.plugin.StopResource(ctx, &pb.StopRequest{ResourceId: resourceID})
	if err != nil {
		return err
	}

	if !resp.Success {
		return fmt.Errorf("failed to stop resource: %s", resp.Error)
	}

	return nil
}

// Producer methods
func (c *GRPCClient) StartProducer(ctx context.Context, resourceID string) (<-chan []byte, <-chan error, error) {
	stream, err := c.producer.Start(ctx, &pb.ProducerStartRequest{ResourceId: resourceID})
	if err != nil {
		return nil, nil, err
	}

	dataCh := make(chan []byte, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(dataCh)
		defer close(errCh)

		for {
			msg, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				errCh <- err
				return
			}

			select {
			case dataCh <- msg.Data:
			case <-ctx.Done():
				return
			}
		}
	}()

	return dataCh, errCh, nil
}

// Consumer methods
// Consumer methods
func (c *GRPCClient) StartConsumer(ctx context.Context, resourceID string) error {
	_, err := c.consumer.Start(ctx, &pb.ConsumerStartRequest{ResourceId: resourceID})
	return err
}

func (c *GRPCClient) SendToConsumer(ctx context.Context, data []byte) error {
	msg := &pb.Message{
		Data:      data,
		Metadata:  make(map[string]string),
		Timestamp: time.Now().UnixNano(),
	}

	_, err := c.consumer.Send(ctx, msg)
	return err
}

// Transformer methods
func (c *GRPCClient) Transform(ctx context.Context, resourceID string, data []byte) ([]byte, error) {
	msg := &pb.Message{
		Data:      data,
		Metadata:  make(map[string]string),
		Timestamp: time.Now().UnixNano(),
	}

	resp, err := c.transformer.Process(ctx, &pb.TransformerProcessRequest{
		ResourceId: resourceID,
		Message:    msg,
	})
	if err != nil {
		return nil, err
	}

	if !resp.Success {
		return nil, fmt.Errorf("transformation failed: %s", resp.Error)
	}

	return resp.Message.Data, nil
}