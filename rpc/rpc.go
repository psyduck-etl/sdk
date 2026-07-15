// Package rpc runs sdk.Plugin implementations as isolated subprocesses,
// hashicorp/go-plugin style. The host and the plugin binary are separate
// executables that only share this package's wire contract (sdk/proto), so
// they no longer need to be compiled with the same toolchain, dependency
// graph, or even the same sdk patch version — the constraint that made
// Go's native -buildmode=plugin loading so brittle.
//
// A plugin binary's main is one line:
//
//	func main() { rpc.Serve(Plugin()) }
//
// The host launches the binary with Dial, which returns an sdk.Plugin
// indistinguishable from an in-process one; pipelines bind and run
// resources over gRPC without knowing it.
package rpc

import (
	"context"
	"fmt"
	"os/exec"

	"github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"

	"github.com/psyduck-etl/sdk"
	"github.com/psyduck-etl/sdk/data"
	"github.com/psyduck-etl/sdk/proto"
)

// Handshake guards against a host launching a binary that isn't a psyduck
// plugin (the magic cookie) or one speaking an incompatible wire contract
// (the protocol version). Bump ProtocolVersion when sdk/proto changes
// incompatibly.
var Handshake = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "PSYDUCK_PLUGIN",
	MagicCookieValue: "8b8ee9e2-psyduck-etl",
}

// pluginName keys the one plugin this package serves in go-plugin's
// plugin-set machinery.
const pluginName = "driver"

// driverPlugin wires the Driver service into go-plugin. Impl is only set
// on the serving (plugin) side; the host side dispenses a client.
type driverPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	impl sdk.Plugin
}

func (p *driverPlugin) GRPCServer(_ *goplugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterDriverServer(s, newServer(p.impl))
	return nil
}

func (p *driverPlugin) GRPCClient(ctx context.Context, _ *goplugin.GRPCBroker, conn *grpc.ClientConn) (any, error) {
	return newClient(ctx, proto.NewDriverClient(conn))
}

// Serve runs p as a plugin subprocess: it serves the Driver gRPC service
// and blocks until the host disconnects. It is the whole body of a plugin
// binary's main.
//
// Serve installs the sdk/data codec chain as the process-wide codec
// factory, so plugins that call sdk.GetCodec resolve the same codecs in a
// subprocess as they did when they were linked into the host.
func Serve(p sdk.Plugin) {
	sdk.RegisterCodecs(data.Codec)
	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: Handshake,
		Plugins:         goplugin.PluginSet{pluginName: &driverPlugin{impl: p}},
		GRPCServer:      goplugin.DefaultGRPCServer,
	})
}

// Client is a handle on a running plugin subprocess. Plugin is live until
// Kill; killing the client tears the subprocess down and invalidates every
// Instance bound through it.
type Client struct {
	client *goplugin.Client
	Plugin sdk.Plugin
}

// Dial launches the plugin binary at path and dispenses its sdk.Plugin. A
// nil logger falls back to go-plugin's default (hclog to stderr); pass a
// leveled logger to control plugin log forwarding.
func Dial(path string, logger hclog.Logger) (*Client, error) {
	client := goplugin.NewClient(&goplugin.ClientConfig{
		HandshakeConfig:  Handshake,
		Plugins:          goplugin.PluginSet{pluginName: &driverPlugin{}},
		Cmd:              exec.Command(path),
		AllowedProtocols: []goplugin.Protocol{goplugin.ProtocolGRPC},
		Logger:           logger,
		Managed:          true,
	})

	rpcClient, err := client.Client()
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dial plugin %s: %w", path, err)
	}

	raw, err := rpcClient.Dispense(pluginName)
	if err != nil {
		client.Kill()
		return nil, fmt.Errorf("dispense plugin %s: %w", path, err)
	}

	p, ok := raw.(sdk.Plugin)
	if !ok {
		client.Kill()
		return nil, fmt.Errorf("dispense plugin %s: unexpected type %T", path, raw)
	}
	return &Client{client: client, Plugin: p}, nil
}

// Kill tears down the plugin subprocess. Safe to call more than once.
func (c *Client) Kill() { c.client.Kill() }

// CleanupClients kills every subprocess Dial has launched. Hosts call this
// on the way out so no plugin outlives its pipeline run.
func CleanupClients() { goplugin.CleanupClients() }
