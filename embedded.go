package nst

import (
	"testing"
	"time"

	"dario.cat/mergo"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/require"
)

// EmbeddedNatsServer represents a nats-server
type EmbeddedNatsServer struct {
	t      testing.TB
	Server *server.Server
	Connections
}

// Shutdown Stops closes all current connections initiated via this API and shutsdown
// the server
func (ts *EmbeddedNatsServer) Shutdown() {
	ts.Connections.Shutdown()
	ts.Server.Shutdown()
}

func (opts *Options) ToNsOptions() *server.Options {
	return &server.Options{
		Port:      opts.Port,
		JetStream: opts.JetStream,
		StoreDir:  opts.StoreDir,
	}
}

func StartInProcessServer(t testing.TB, opts *Options) NatsServer {
	defaults := DefaultNatsServerOptions()
	if opts == nil {
		opts = DefaultNatsServerOptions()
	}

	so := opts.ToNsOptions()
	err := mergo.Merge(so, defaults.ToNsOptions())
	require.NoError(t, err)

	if opts.ConfigFile != "" {
		require.NoError(t, so.ProcessConfigFile(opts.ConfigFile))
	}

	s, err := server.NewServer(so)
	require.NoError(t, err)

	go s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		t.Fatal("Unable to start NATS Server in Go Routine")
	}

	pi := s.PortsInfo(10 * time.Second)
	var ports ConnectionPorts
	if len(pi.Nats) > 0 {
		ports.Nats = pi.Nats
	}
	if len(pi.WebSocket) > 0 {
		ports.WebSocket = pi.WebSocket
	}

	return &EmbeddedNatsServer{
		t:           t,
		Server:      s,
		Connections: Connections{ConnectionPorts: ports, t: t},
	}
}
