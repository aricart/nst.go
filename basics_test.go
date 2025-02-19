package nst

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/nats-io/nkeys"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	authb "github.com/synadia-io/jwt-auth-builder.go"
	"github.com/synadia-io/jwt-auth-builder.go/providers/nsc"
)

type BasicTestSuite struct {
	suite.Suite
}

func TestBasicsSuite(t *testing.T) {
	suite.Run(t, new(BasicTestSuite))
}

func (s *BasicTestSuite) TestDirBasics() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()
	s.T().Log(td)
	td.WriteFile("server.conf", []byte("hello"))
	data := td.ReadFile("server.conf")
	require.Equal(s.T(), data, []byte("hello"))
}

func (s *BasicTestSuite) TestSimpleServer() {
	dir := NewTestDir(s.T(), "", "nst-test")
	defer dir.Cleanup()

	ns := NewNatsServer(dir, nil)
	defer ns.Shutdown()

	nc := ns.RequireConnect()
	defer nc.Close()
	_, err := nc.Subscribe("echo", func(m *nats.Msg) {
		_ = m.Respond(m.Data)
	})
	s.NoError(err)

	r, err := nc.Request("echo", []byte("hello"), 2*time.Second)
	s.NoError(err)
	s.Equal(r.Data, []byte("hello"))
}

func (s *BasicTestSuite) TestShutdownClosesClients() {
	dir := NewTestDir(s.T(), "", "nst-test")
	defer dir.Cleanup()

	ns := NewNatsServer(dir, nil)
	nc := ns.RequireConnect()
	ns.Shutdown()
	s.True(nc.IsClosed())
}

func (s *BasicTestSuite) TestServerConfig() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()

	u := User{User: "a", Password: "b", Permissions: &Permissions{}}
	u.Permissions.Pub.Allow.Add("echo")
	u.Permissions.Pub.Allow.Add(UserInfoSubj)
	u.Permissions.Sub.Allow.Add("_INBOX.>")

	conf := Conf{}
	conf.Debug = true
	conf.Authorization.Users.Add(u)
	conf.WebSocket = &WebSocket{
		Port:  -1,
		NoTls: true,
	}

	fn := td.WriteFile("server.conf", conf.Marshal(s.T()))

	ns := NewNatsServer(td, &Options{
		ConfigFile: fn,
	})
	defer ns.Shutdown()

	nc := ns.RequireConnect(nats.UserInfo("a", "b"))
	defer nc.Close()

	ws, err := ns.WsMaybeConnect(nats.UserInfo("a", "b"))
	s.NoError(err)
	s.NotNil(ws)
	s.Contains(ws.Servers()[0], "ws://127.0.0.1:")

	info := ClientInfo(s.T(), nc)
	s.Len(info.Data.Permissions.Pub.Allow, 2)
	s.Contains(info.Data.Permissions.Pub.Allow, "echo")
	s.Contains(info.Data.Permissions.Pub.Allow, UserInfoSubj)
	s.Len(info.Data.Permissions.Sub.Allow, 1)
	s.Contains(info.Data.Permissions.Sub.Allow, "_INBOX.>")
	s.False(info.Data.Permissions.AllowResponses)
}

func (s *BasicTestSuite) TestServerJetStreamServerConfig() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()

	conf := Conf{}
	conf.JetStream = &JetStream{
		StoreDir: fmt.Sprintf("%s/js", td.Dir),
	}
	fn := td.WriteFile("server.conf", conf.Marshal(s.T()))

	ns := NewNatsServer(td, &Options{ConfigFile: fn})
	defer ns.Shutdown()

	fi, err := os.Stat(filepath.Join(td.Dir, "js"))
	s.NoError(err)
	s.True(fi.IsDir())
}

func (s *BasicTestSuite) TestServerLeafNodeConfig() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()

	conf := Conf{}
	conf.LeafNodes = &LeafNodes{
		Port: 7422,
	}
	fn := td.WriteFile("server.conf", conf.Marshal(s.T()))
	ns := NewNatsServer(td, &Options{ConfigFile: fn})
	defer ns.Shutdown()

	nc := ns.RequireConnect()
	nc.Subscribe("q", func(m *nats.Msg) {
		_ = m.Respond(m.Data)
	})

	ln := Conf{}
	ln.LeafNodes = &LeafNodes{}

	ln.LeafNodes.Remotes = append(ln.LeafNodes.Remotes,
		Remote{Urls: []string{"nats://127.0.0.1:7422"}})

	fn = td.WriteFile("leafnode.conf", ln.Marshal(s.T()))
	leaf := NewNatsServer(td, &Options{ConfigFile: fn})
	defer leaf.Shutdown()

	lc := leaf.RequireConnect()
	r, err := lc.Request("q", []byte("hello"), 2*time.Second)
	s.NoError(err)
	s.Equal(r.Data, []byte("hello"))
}

func (s *BasicTestSuite) TestServerConfigAccounts() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()

	conf := Conf{Accounts: make(map[string]Account)}
	conf.Accounts["B"] = Account{}
	conf.Authorization.Users.Add(User{User: "auth", Password: "pwd"})
	akp, err := nkeys.CreateAccount()
	s.NoError(err)
	pub, err := akp.PublicKey()
	s.NoError(err)
	conf.Authorization.AuthCallout = &AuthCallout{}
	conf.Authorization.AuthCallout.Issuer = pub
	conf.Authorization.AuthCallout.AuthUsers.Add("auth")

	// s.T().Log(string(conf.Marshal(s.T())))

	fn := td.WriteFile("server.conf", conf.Marshal(s.T()))

	ns := NewNatsServer(td, &Options{
		ConfigFile: fn,
	})
	defer ns.Shutdown()

	nc := ns.RequireConnect(nats.UserInfo("auth", "pwd"))
	r, err := nc.Request("$SYS.REQ.USER.INFO", []byte{}, time.Second*2)
	s.NoError(err)
	var info UserInfo
	s.NoError(json.Unmarshal(r.Data, &info))
	fmt.Printf("%+v\n", info)
}

func (s *BasicTestSuite) TestSys() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()

	conf := Conf{Accounts: make(map[string]Account)}
	sys := "SYS"
	conf.SystemAccount = &sys
	conf.Accounts[sys] = Account{}

	fn := td.WriteFile("server.conf", conf.Marshal(s.T()))

	ns := NewNatsServer(td, &Options{
		ConfigFile: fn,
	})
	defer ns.Shutdown()
}

func (s *BasicTestSuite) TestServerConfigJetStream() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()

	opts := DefaultNatsServerWithJetStreamOptions(td.Dir)

	ns := NewNatsServer(td, opts)
	defer ns.Shutdown()

	nc := ns.RequireConnect()
	defer nc.Close()
	js, err := jetstream.New(nc)
	s.NoError(err)

	_, err = js.CreateStream(context.Background(), jetstream.StreamConfig{
		Name:     "hello",
		Subjects: []string{"hello"},
	})
	s.NoError(err)
}

func (s *BasicTestSuite) TestOperator() {
	t := s.T()
	td := NewTestDir(t, "", "nst-test")
	defer td.Cleanup()

	auth, err := authb.NewAuth(nsc.NewNscProvider(fmt.Sprintf("%s/nsc/stores", td.Dir), fmt.Sprintf("%s/nsc/keys", td.Dir)))
	s.NoError(err)

	o, err := auth.Operators().Add("O")
	s.NoError(err)

	sys, err := o.Accounts().Add("SYS")
	s.NoError(err)
	s.NoError(o.SetSystemAccount(sys))

	a, err := o.Accounts().Add("A")
	s.NoError(err)

	resolver := ResolverFromAuth(t, o)

	ns := NewNatsServer(td, &Options{
		Debug:      true,
		ConfigFile: td.WriteFile("server.conf", resolver.Marshal(t)),
	})

	u, err := a.Users().Add("a", "")
	s.NoError(err)

	creds, err := u.Creds(time.Hour)
	s.NoError(err)
	nc, err := ns.MaybeConnect(nats.UserCredentials(td.WriteFile("a.creds", creds)))
	s.NoError(err)
	defer nc.Close()

	defer ns.Shutdown()
}

func TestPush(t *testing.T) {
	td := NewTestDir(t, "", "nst-test")
	defer td.Cleanup()

	auth, err := authb.NewAuth(nsc.NewNscProvider(fmt.Sprintf("%s/nsc/stores", td.Dir), fmt.Sprintf("%s/nsc/keys", td.Dir)))
	require.NoError(t, err)

	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	sys, err := o.Accounts().Add("SYS")
	require.NoError(t, err)
	export, err := sys.Exports().Services().Add("resolver", "$SYS.REQ.CLAIMS.*")
	require.NoError(t, err)
	require.NoError(t, export.SetTokenRequired(true))
	require.NoError(t, o.SetSystemAccount(sys))

	a, err := o.Accounts().Add("A")
	require.NoError(t, err)
	si, err := export.GenerateImport()
	require.NoError(t, err)
	token, err := export.GenerateActivation(a.Subject(), sys.Subject())
	require.NoError(t, err)
	require.NoError(t, si.SetToken(token))
	require.NoError(t, a.Imports().Services().AddWithConfig(si))

	require.NoError(t, auth.Commit())

	config := ResolverFromAuth(t, o)
	config.Resolver.Type = FullResolver
	config.Resolver.Dir = filepath.Join(td.Dir, "jwts")
	config.Resolver.AllowDelete = true
	config.Resolver.UpdateInterval = "60s"
	config.Resolver.Timeout = "2s"

	ns := NewNatsServer(td, &Options{
		ConfigFile: td.WriteFile("server.conf", config.Marshal(t)),
	})
	defer ns.Shutdown()

	sysU, err := sys.Users().Add("sys", "")
	require.NoError(t, err)
	d, err := sysU.Creds(time.Hour)
	sysNc := ns.RequireConnect(nats.UserCredentials(td.WriteFile("sys.creds", d)))

	u, err := a.Users().Add("a", "")
	require.NoError(t, err)
	d, err = u.Creds(time.Hour)
	require.NoError(t, err)
	nc := ns.RequireConnect(nats.UserCredentials(td.WriteFile("a.creds", d)))
	defer nc.Close()

	// list
	list, err := ListAccounts(sysNc)
	require.NoError(t, err)
	require.Len(t, list.Accounts, 2)
	t.Logf("%+v", list)

	c, err := o.Accounts().Add("C")
	require.NoError(t, err)

	ur, err := UpdateAccount(sysNc, c.JWT())
	require.NoError(t, err)
	require.Equal(t, 200, ur.UpdateData.Code)
	t.Logf("%+v", ur)

	// connect user from account c
	uc, err := c.Users().Add("c", "")
	require.NoError(t, err)
	d, err = uc.Creds(time.Hour)
	require.NoError(t, err)

	nc2 := ns.RequireConnect(nats.UserCredentials(td.WriteFile("c.creds", d)))
	t.Log(nc2.ConnectedUrl())
	nc2.Close()

	lr, err := ListAccounts(sysNc)
	require.NoError(t, err)

	require.Contains(t, lr.Accounts, sys.Subject())
	require.Contains(t, lr.Accounts, c.Subject())
	require.Contains(t, lr.Accounts, a.Subject())

	token, err = GetAccount(sysNc, c.Subject())
	require.NoError(t, err)
	cc, err := jwt.DecodeAccountClaims(token)
	require.NoError(t, err)
	require.Equal(t, cc.Subject, c.Subject())

	token, err = DeleteRequestToken(o, o.Subject(), c.Subject())
	require.NoError(t, err)
	// this will not delete if the server doesn't have https://github.com/nats-io/nats-server/pull/6427
	_, err = DeleteAccount(sysNc, token)
	require.NoError(t, err)
}

func TestConf(t *testing.T) {
	td := NewTestDir(t, "", "nst-test")
	conf := Conf{
		PortsFileDir: td.Dir,
		Debug:        true,
		Trace:        false,
		Port:         2224,
		Accounts: Accounts{
			"A": Account{
				Users: Users{
					User{User: "a", Password: "b", Permissions: &Permissions{
						Pub: AllowDeny{
							Allow: []string{"foo"},
							Deny:  []string{"bar"},
						},
						Sub: AllowDeny{
							Allow: []string{"q"},
							Deny:  []string{"qq"},
						},
						AllowResponses: true,
					}},
				},
			},
		},
		JetStream: &JetStream{
			StoreDir: "/tmp/jsstoredir",
		},
		LeafNodes: &LeafNodes{
			Port: 7422,
			Remotes: []Remote{
				{
					Urls: []string{"nats://127.0.0.1:7422"},
				},
			},
		},
		WriteDeadline: "3s",
		WebSocket: &WebSocket{
			Port:  -1,
			NoTls: true,
		},
		MonitoringPort: 1234,
	}

	fn := td.WriteFile("server.conf", conf.Marshal(t))

	d, err := os.ReadFile(fn)
	require.NoError(t, err)

	var conf2 Conf
	require.NoError(t, json.Unmarshal(d, &conf2))

	require.Equal(t, conf.PortsFileDir, conf2.PortsFileDir)
	require.Equal(t, conf.Debug, conf2.Debug)
	require.Equal(t, conf.Trace, conf2.Trace)
	require.Equal(t, conf.Port, conf2.Port)

	require.Equal(t, len(conf.Accounts), len(conf2.Accounts))
	A := conf.Accounts["A"]
	AA := conf2.Accounts["A"]
	require.NotNil(t, AA)
	U := A.Users
	UU := AA.Users
	require.NotNil(t, UU)
	require.Equal(t, len(U), len(UU))

	u := UU[0]
	require.Equal(t, u.User, "a")
	require.Equal(t, u.Password, "b")
	// these testing that the serialization wrote Allow/Deny - expression of one
	// makes the other one...
	require.Contains(t, u.Permissions.Pub.Allow, "foo")
	require.Contains(t, u.Permissions.Pub.Deny, "bar")
	require.Contains(t, u.Permissions.Sub.Allow, "q")
	require.Contains(t, u.Permissions.Sub.Deny, "qq")
	require.Equal(t, u.Permissions.AllowResponses, true)

	require.NotNil(t, conf2.JetStream)
	require.Equal(t, conf2.JetStream.StoreDir, "/tmp/jsstoredir")

	require.NotNil(t, conf2.LeafNodes)
	require.Equal(t, conf2.LeafNodes.Port, 7422)
	require.Equal(t, len(conf2.LeafNodes.Remotes), 1)
	require.Equal(t, conf2.LeafNodes.Remotes[0].Urls[0], "nats://127.0.0.1:7422")

	require.Equal(t, conf2.WriteDeadline, "3s")

	require.Equal(t, conf2.WebSocket.Port, -1)
	require.Equal(t, conf2.WebSocket.NoTls, true)

	require.Equal(t, conf2.MonitoringPort, 1234)

	ns := StartInProcessServer(t, &Options{
		ConfigFile: fn,
		InProcess:  true,
	})
	nc := ns.RequireConnect(nats.UserInfo("a", "b"))
	require.NoError(t, nc.Flush())
	ns.Shutdown()

	ns = StartExternalProcessWithConfig(t, fn)
	ns.RequireConnect(nats.UserInfo("a", "b"))
	ns.Shutdown()
}
