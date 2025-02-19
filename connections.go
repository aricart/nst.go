package nst

import (
	"errors"
	"sync"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

type ConnectionPorts struct {
	Nats      []string
	WebSocket []string
}

type Connections struct {
	t testing.TB
	ConnectionPorts
	sync.Mutex
	Conns []*nats.Conn
}

func (ts *Connections) NatsURLs() []string {
	return ts.ConnectionPorts.Nats
}

func (ts *Connections) WsURLs() []string {
	return ts.ConnectionPorts.WebSocket
}

func (ts *Connections) Shutdown() {
	ts.Lock()
	defer ts.Unlock()
	for _, c := range ts.Conns {
		c.Close()
	}
}

func (ts *Connections) TrackConn(conn ...*nats.Conn) {
	ts.Lock()
	defer ts.Unlock()
	ts.Conns = append(ts.Conns, conn...)
}

// RequireConnect returns a connection, the server must not have auth enabled
func (ts *Connections) RequireConnect(opts ...nats.Option) *nats.Conn {
	nc, err := ts.UntrackedConnection(opts...)
	require.NoError(ts.t, err)
	ts.TrackConn(nc)
	return nc
}

// MaybeConnect this connection could fail and tests want to verify it
func (ts *Connections) MaybeConnect(options ...nats.Option) (*nats.Conn, error) {
	ts.Lock()
	defer ts.Unlock()
	if len(ts.ConnectionPorts.Nats) == 0 {
		return nil, errors.New("tcp port not enabled")
	}
	nc, err := nats.Connect(ts.ConnectionPorts.Nats[0], options...)
	if err == nil {
		ts.Conns = append(ts.Conns, nc)
	}
	return nc, err
}

func (ts *Connections) WsMaybeConnect(opts ...nats.Option) (*nats.Conn, error) {
	if len(ts.ConnectionPorts.WebSocket) == 0 {
		return nil, errors.New("websocket port not enabled")
	}
	ts.Lock()
	defer ts.Unlock()
	nc, err := nats.Connect(ts.ConnectionPorts.WebSocket[0], opts...)
	if err == nil {
		ts.Conns = append(ts.Conns, nc)
	}
	return nc, err
}

func (ts *Connections) UntrackedConnection(opts ...nats.Option) (*nats.Conn, error) {
	if len(ts.ConnectionPorts.Nats) == 0 {
		return nil, errors.New("tcp port not enabled")
	}
	return nats.Connect(ts.ConnectionPorts.Nats[0], opts...)
}
