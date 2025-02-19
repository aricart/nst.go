package nst

import (
	"path/filepath"

	"dario.cat/mergo"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
)

const UserInfoSubj = "$SYS.REQ.USER.INFO"

type NatsServer interface {
	// RequireConnect creates a connection that is not expected to fail, if it fail test will fail
	RequireConnect(opts ...nats.Option) *nats.Conn
	// MaybeConnect attempts to create a TCP connection
	MaybeConnect(options ...nats.Option) (*nats.Conn, error)
	// WsMaybeConnect attempts to create a connection via WebSocket.
	WsMaybeConnect(opts ...nats.Option) (*nats.Conn, error)
	// TrackConn simply adds a connection made to the server via direct client API to be discarded on ShutDown
	TrackConn(conn ...*nats.Conn)
	// UntrackedConnection convenience function to create a connection that is not tracked
	UntrackedConnection(opts ...nats.Option) (*nats.Conn, error)
	// Shutdown stop the server closing all connections initiated via the API if tracked.
	Shutdown()
	// NatsURLs returns a list of all the connection URLs (tcp)
	NatsURLs() []string
	// WsURLs returns a list of all the connection URLs (tcp)
	WsURLs() []string
}

type Options struct {
	Debug      bool
	Trace      bool
	ConfigFile string
	Port       int
	JetStream  bool
	StoreDir   string
	InProcess  bool
}

func DefaultNatsServerOptions() *Options {
	return &Options{
		Port: -1,
	}
}

// DefaultNatsServerWithJetStreamOptions basic config for supporting JetStream
func DefaultNatsServerWithJetStreamOptions(tempDir string) *Options {
	opts := DefaultNatsServerOptions()
	opts.JetStream = true
	opts.StoreDir = tempDir
	return opts
}

func NewNatsServer(dir *TestDir, opts *Options) NatsServer {
	t := dir.t
	// sanity in the options
	if opts == nil {
		opts = DefaultNatsServerOptions()
	}
	if err := mergo.Merge(opts, DefaultNatsServerOptions()); err != nil {
		require.NoError(t, err)
	}
	if opts.JetStream && opts.StoreDir == "" {
		opts.StoreDir = filepath.Join(dir.Dir, "jetstream")
	}

	var conf *ResolverConf
	if opts.ConfigFile != "" {
		conf = ParseConf(t, opts.ConfigFile)
	} else {
		conf = &ResolverConf{}
	}
	conf.Port = opts.Port

	if opts.Debug {
		conf.Debug = true
	}
	if opts.Trace {
		conf.Trace = true
	}
	if opts.JetStream {
		conf.JetStream = &JetStream{
			StoreDir: opts.StoreDir,
		}
	}
	opts.ConfigFile = dir.WriteFile("server.conf", conf.Marshal(t))

	if opts.InProcess {
		return StartInProcessServer(t, opts)
	} else {
		return StartExternalProcessWithConfig(t, opts.ConfigFile)
	}
}

//func StartNatsServer(t testing.TB, opts *Options) (NatsServer, string) {
//	if opts.InProcess {
//		return StartInProcessServer(t, opts)
//	} else {
//		StartExternalProcess(t, opts)
//	}
//}
