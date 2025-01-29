package nst

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	authb "github.com/synadia-io/jwt-auth-builder.go"

	"dario.cat/mergo"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

const UserInfoSubj = "$SYS.REQ.USER.INFO"

// NatsServer represents a nats-server
type NatsServer struct {
	sync.Mutex
	t      testing.TB
	Server *server.Server
	// Resolver *ResolverConf
	Url   string
	Conns []*nats.Conn
}

//func NewNatsServerWithResolverConfig(t *testing.T, opts *server.Options) *NatsServer {
//	if opts != nil && opts.ConfigFile != "" {
//		t.Fatal("config file option is not valid when using the resolver")
//	}
//
//	tempDir, err := os.MkdirTemp(os.TempDir(), "callout_test")
//	require.NoError(t, err)
//	t.Log(tempDir)
//
//	if opts == nil {
//		opts = DefaultNatsServerOptions()
//	}
//
//	rc := NewResolverConfig(t, tempDir)
//	config := rc.Store(tempDir)
//	opts.ConfigFile = config
//
//	ns, u := SetupNatsServerUsingDir(t, opts, tempDir)
//	return &NatsServer{
//		t:        t,
//		Server:   ns,
//		Url:      u,
//		Resolver: rc,
//	}
//}

func NewNatsServer(t testing.TB, opts *server.Options) *NatsServer {
	ns, u := StartNatsServer(t, opts)
	return &NatsServer{
		t:      t,
		Server: ns,
		Url:    u,
	}
}

func StartNatsServer(t testing.TB, opts *server.Options) (*server.Server, string) {
	defaults := DefaultNatsServerOptions()
	if opts == nil {
		opts = DefaultNatsServerOptions()
	}

	err := mergo.Merge(opts, defaults)
	require.NoError(t, err)

	if opts.ConfigFile != "" {
		require.NoError(t, opts.ProcessConfigFile(opts.ConfigFile))
	}

	s, err := server.NewServer(opts)
	require.NoError(t, err)

	go s.Start()
	if !s.ReadyForConnections(10 * time.Second) {
		t.Fatal("Unable to start NATS Server in Go Routine")
	}

	ports := s.PortsInfo(10 * time.Second)

	return s, ports.Nats[0]
}

// DefaultNatsServerWithJetStreamOptions basic config for supporting JetStream
func DefaultNatsServerWithJetStreamOptions(tempDir string) *server.Options {
	opts := DefaultNatsServerOptions()
	opts.JetStream = true
	opts.StoreDir = tempDir
	return opts
}

// DefaultNatsServerOptions returns a core NATS configuration
func DefaultNatsServerOptions() *server.Options {
	return &server.Options{
		Debug:                 true,
		Trace:                 true,
		Host:                  "127.0.0.1",
		Port:                  -1,
		NoLog:                 false,
		NoSigs:                true,
		MaxControlLine:        4096,
		DisableShortFirstPing: true,
	}
}

func (ts *NatsServer) TrackConn(conn ...*nats.Conn) {
	ts.Lock()
	defer ts.Unlock()
	ts.Conns = append(ts.Conns, conn...)
}

// RequireConnect returns a connection, the server must not have auth enabled
func (ts *NatsServer) RequireConnect(opts ...nats.Option) *nats.Conn {
	nc, err := ts.UntrackedConnection(opts...)
	require.NoError(ts.t, err)
	ts.TrackConn(nc)
	return nc
}

// MaybeConnect this connection could fail and tests want to verify it
func (ts *NatsServer) MaybeConnect(options ...nats.Option) (*nats.Conn, error) {
	ts.Lock()
	defer ts.Unlock()
	nc, err := nats.Connect(ts.Url, options...)
	if err == nil {
		ts.Conns = append(ts.Conns, nc)
	}
	return nc, err
}

func (ts *NatsServer) WsMaybeConnect(opts ...nats.Option) (*nats.Conn, error) {
	ts.Lock()
	defer ts.Unlock()
	var ws string
	pi := ts.Server.PortsInfo(10 * time.Second)
	if len(pi.WebSocket) > 0 {
		ws = pi.WebSocket[0]
	} else {
		return nil, errors.New("websocket not enabled")
	}

	nc, err := nats.Connect(ws, opts...)
	if err == nil {
		ts.Conns = append(ts.Conns, nc)
	}
	return nc, err
}

func (ts *NatsServer) UntrackedConnection(opts ...nats.Option) (*nats.Conn, error) {
	return nats.Connect(ts.Url, opts...)
}

//func (ts *NatsServer) ConnectAccount(account string, user string, bearer bool) (*nats.Conn, error) {
//	u := ts.Resolver.Identities.CreateUser(account, user, bearer)
//	return ts.MaybeConnect(u.ConnectOptions())
//}

// Shutdown Stops closes all current connections initiated via this API and shutsdown
// the server
func (ts *NatsServer) Shutdown() {
	ts.Lock()
	defer ts.Unlock()
	for _, c := range ts.Conns {
		c.Close()
	}
	ts.Server.Shutdown()
}

func ClientInfo(t testing.TB, nc *nats.Conn) UserInfo {
	r, err := nc.Request(UserInfoSubj, nil, time.Second*2)
	require.NoError(t, err)
	require.NotNil(t, r)
	var info UserInfo
	require.NoError(t, json.Unmarshal(r.Data, &info))
	return info
}

//func (ts *NatsServer) NewKv(bucket string) jetstream.KeyValue {
//	nc := ts.RequireConnect()
//	js, err := jetstream.New(nc)
//	require.NoError(ts.t, err)
//
//	kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
//		Bucket: bucket,
//	})
//	require.NoError(ts.t, err)
//	return kv
//}

type ErrorDetails struct {
	Account     string `json:"account"`
	Code        int    `json:"code"`
	Description string `json:"description"`
}
type ServerDetails struct {
	Name      string    `json:"name"`
	Host      string    `json:"host"`
	ID        string    `json:"id"`
	Version   string    `json:"ver"`
	Jetstream bool      `json:"jetstream"`
	Flags     int       `json:"flags"`
	Sequence  int       `json:"seq"`
	Time      time.Time `json:"time"`
}

type UserData struct {
	User        string      `json:"user"`
	Account     string      `json:"account"`
	Permissions Permissions `json:"permissions"`
	Expires     int64       `json:"expires"`
}

type UserInfo struct {
	ServerDetails
	Data UserData `json:"data"`
}

type UpdateData struct {
	Account string `json:"account"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type ResolverResponse struct {
	Error  *ErrorDetails `json:"error,omitempty"`
	Server ServerDetails `json:"server"`
}

type ResolverUpdateResponse struct {
	ResolverResponse
	UpdateData UpdateData `json:"data"`
}

type ResolverListResponse struct {
	ResolverResponse
	Accounts []string `json:"data"`
}

func DeleteRequestToken(operator authb.Operator, key string, account ...string) (string, error) {
	r := jwt.NewGenericClaims(key)
	r.Data = make(map[string]interface{})
	r.Data["accounts"] = account
	return operator.IssueClaim(r, key)
}

func resolverRequest(nc *nats.Conn, subj string, payload string, resp any) error {
	m, err := nc.Request(subj, []byte(payload), time.Second*2)
	if err != nil {
		return err
	}
	return json.Unmarshal(m.Data, resp)
}

func UpdateAccount(nc *nats.Conn, token string) (*ResolverUpdateResponse, error) {
	var r ResolverUpdateResponse
	err := resolverRequest(nc, "$SYS.REQ.CLAIMS.UPDATE", token, &r)
	return &r, err
}

// DeleteAccount will only work if https://github.com/nats-io/nats-server/pull/6427 is merged
func DeleteAccount(nc *nats.Conn, token string) (*ResolverResponse, error) {
	var r ResolverResponse
	err := resolverRequest(nc, "$SYS.REQ.CLAIMS.DELETE", token, &r)
	return &r, err
}

func ListAccounts(nc *nats.Conn) (*ResolverListResponse, error) {
	var r ResolverListResponse
	err := resolverRequest(nc, "$SYS.REQ.CLAIMS.LIST", "", &r)
	return &r, err
}

func GetAccount(nc *nats.Conn, id string) (string, error) {
	m, err := nc.Request(fmt.Sprintf("$SYS.REQ.ACCOUNT.%s.CLAIMS.LOOKUP", id), nil, time.Second*2)
	if err != nil {
		return "", err
	}
	return string(m.Data), err
}
