package nst

import (
	"encoding/json"
	"sync"
	"testing"
	"time"

	"dario.cat/mergo"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

const UserInfoSubj = "$SYS.REQ.USER.INFO"

// NatsServer represents a nats-server
type NatsServer struct {
	sync.Mutex
	t      *testing.T
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

func NewNatsServer(t *testing.T, opts *server.Options) *NatsServer {
	ns, u := StartNatsServer(t, opts)
	return &NatsServer{
		t:      t,
		Server: ns,
		Url:    u,
	}
}

func StartNatsServer(t *testing.T, opts *server.Options) (*server.Server, string) {
	defaults := DefaultNatsServerOptions()
	if opts == nil {
		opts = DefaultNatsServerOptions()
	}

	err := mergo.Merge(defaults, opts)
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

	ports := s.PortsInfo(time.Second)

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

// Connect returns a connection, the server must not have auth enabled
func (ts *NatsServer) Connect() *nats.Conn {
	nc, err := ts.MaybeConnect(nil)
	require.NoError(ts.t, err)
	return nc
}

//func (ts *NatsServer) ConnectAccount(account string, user string, bearer bool) (*nats.Conn, error) {
//	u := ts.Resolver.Identities.CreateUser(account, user, bearer)
//	return ts.MaybeConnect(u.ConnectOptions())
//}

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

// ConnectWithOptions expect this connection to work
func (ts *NatsServer) ConnectWithOptions(options nats.Options) *nats.Conn {
	ts.Lock()
	defer ts.Unlock()
	options.Url = ts.Url
	nc, err := options.Connect()
	require.NoError(ts.t, err)
	if err == nil {
		ts.Conns = append(ts.Conns, nc)
	}
	return nc
}

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

func ClientInfo(t *testing.T, nc *nats.Conn) UserInfo {
	r, err := nc.Request(UserInfoSubj, nil, time.Second*2)
	require.NoError(t, err)
	require.NotNil(t, r)
	var info UserInfo
	require.NoError(t, json.Unmarshal(r.Data, &info))
	return info
}

//func (ts *NatsServer) NewKv(bucket string) jetstream.KeyValue {
//	nc := ts.Connect()
//	js, err := jetstream.New(nc)
//	require.NoError(ts.t, err)
//
//	kv, err := js.CreateKeyValue(context.Background(), jetstream.KeyValueConfig{
//		Bucket: bucket,
//	})
//	require.NoError(ts.t, err)
//	return kv
//}

//	type ErrorDetails struct {
//		Account     string `json:"account"`
//		Code        int    `json:"code"`
//		Description string `json:"description"`
//	}
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

//
//type PushResponse struct {
//	Error  *ErrorDetails `json:"error,omitempty"`
//	Server ServerDetails `json:"server"`
//}

//func (ts *NatsServer) AddAccount(name string) {
//	if ts.Resolver == nil {
//		ts.t.Fatal("AddAccount only works with resolver configurations")
//	}
//	ts.Resolver.NewAccount(name)
//	ts.pushAccount(ts.t, name)
//}
//
//func (ts *NatsServer) pushAccount(t *testing.T, name string) {
//	a := ts.Resolver.Identities.Accounts[name]
//	require.NotNil(t, a)
//
//	nc, err := ts.ConnectAccount("SYS", "sys", false)
//	require.NoError(t, err)
//	defer nc.Close()
//
//	m, err := nc.Request("$SYS.REQ.CLAIMS.UPDATE", []byte(a.Token), time.Second*2)
//	require.NoError(t, err)
//	require.NotNil(t, m)
//	require.NotEmpty(t, m.Data)
//
//	var v PushResponse
//	err = json.Unmarshal(m.Data, &v)
//	require.NoError(t, err)
//	require.Nil(t, v.Error)
//}
