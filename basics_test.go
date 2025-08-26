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
	require.NoError(t, err)
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

	ns := StartExternalProcessWithConfig(t, fn)
	ns.RequireConnect(nats.UserInfo("a", "b"))
	ns.Shutdown()
}

func TestUnboundCallout(t *testing.T) {
	td := NewTestDir(t, "", "nst-test")
	defer td.Cleanup()

	auth, err := authb.NewAuth(nsc.NewNscProvider(fmt.Sprintf("%s/nsc/stores", td.Dir), fmt.Sprintf("%s/nsc/keys", td.Dir)))
	require.NoError(t, err)

	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	SYS, err := o.Accounts().Add("SYS")
	require.NoError(t, err)
	require.NoError(t, o.SetSystemAccount(SYS))

	A, err := o.Accounts().Add("A")
	require.NoError(t, err)
	a, err := A.Users().Add("default_user", "")
	require.NoError(t, err)
	require.NoError(t, a.SetBearerToken(true))
	require.NoError(t, a.PubPermissions().SetAllow("$SYS.REQ.USER.INFO"))
	require.NoError(t, a.SubPermissions().SetAllow("_INBOX.>"))

	require.NoError(t, auth.Commit())

	config := ResolverFromAuth(t, o)
	config.Resolver.Type = MemResolver

	config.DefaultSentinel = a.JWT()
	ns := NewNatsServer(td, &Options{
		ConfigFile: td.WriteFile("server.conf", config.Marshal(t)),
	})
	defer ns.Shutdown()

	nc := ns.RequireConnect()
	defer nc.Close()

	i := ClientInfo(t, nc)
	require.Equal(t, i.Data.User, a.Subject())
}

func TestDefaultSentinel(t *testing.T) {
	td := NewTestDir(t, "", "nst-test")
	defer td.Cleanup()

	auth, err := authb.NewAuth(nsc.NewNscProvider(fmt.Sprintf("%s/nsc/stores", td.Dir), fmt.Sprintf("%s/nsc/keys", td.Dir)))
	require.NoError(t, err)

	o, err := auth.Operators().Add("O")
	require.NoError(t, err)

	SYS, err := o.Accounts().Add("SYS")
	require.NoError(t, err)
	require.NoError(t, o.SetSystemAccount(SYS))

	A, err := o.Accounts().Add("A")
	require.NoError(t, err)
	a, err := A.Users().Add("default_user", "")
	require.NoError(t, err)
	require.NoError(t, a.SetBearerToken(true))
	require.NoError(t, a.PubPermissions().SetAllow("$SYS.REQ.USER.INFO"))
	require.NoError(t, a.SubPermissions().SetAllow("_INBOX.>"))

	require.NoError(t, auth.Commit())

	config := ResolverFromAuth(t, o)
	config.Debug = true
	config.Trace = true
	config.Resolver.Type = MemResolver

	config.DefaultSentinel = a.JWT()
	ns := NewNatsServer(td, &Options{
		ConfigFile: td.WriteFile("server.conf", config.Marshal(t)),
	})
	defer ns.Shutdown()

	nc, err := ns.MaybeConnect()
	require.NoError(t, err)
	require.NoError(t, nc.Flush())

	i := ClientInfo(t, nc)
	require.Equal(t, i.Data.User, a.Subject())
}

func TestClustering(t *testing.T) {
	td := NewTestDir(t, "", "nst-test")
	defer td.Cleanup()

	var conf Conf
	conf.Port = -1
	conf.Cluster = &Cluster{
		Port: -1,
	}

	srv := NewNatsServer(td, &Options{
		ConfigFile: td.WriteFile("server.conf", conf.Marshal(t)),
	})

	defer srv.Shutdown()

	nc := srv.RequireConnect()
	nc.Subscribe("hello", func(msg *nats.Msg) {
		msg.Respond(nil)
	})

	var conf2 Conf
	conf2.Port = -1
	conf2.Cluster = &Cluster{
		Port:   -1,
		Routes: srv.ClusterURLs(),
	}

	srv2 := NewNatsServer(td, &Options{
		ConfigFile: td.WriteFile("server2.conf", conf2.Marshal(t)),
	})
	defer srv2.Shutdown()

	nc2 := srv2.RequireConnect()

	_, err := nc2.Request("hello", nil, time.Second*2)
	require.NoError(t, err)
}

func TestParseResolverConfJSON(t *testing.T) {
	jsonData := `{
		"default_sentinel": "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiIyR1FRQjdTUlJOUklRNVIyRUxTVklSWFlRQzZBQTVRV1IyUlhRTFlHUklINVZINE1LVENBIiwiaWF0IjoxNzU2MjM1MTUwLCJpc3MiOiJBQkQ1N0dTNVdVTFdUTlZBVzZGVUhUS0xRN1VJWUlBWVBLSFFKNlBKRkNZUlk3TVJYNzdaRURERiIsIm5hbWUiOiJzZW50aW5lbCIsInN1YiI6IlVDNzNLMjVLRjZPWlpRVVhIQlZSS0hMUklRRlU1NEdJUUk0TlFYMlNBSFdBVU9CSDY3UTRWSTVZIiwibmF0cyI6eyJwdWIiOnt9LCJzdWIiOnt9LCJpc3N1ZXJfYWNjb3VudCI6IkFEQjc3STJQQ0pTUkVBUlc2NFhWUFFUM1Q1NU5QUlZSWDM3VUJHN0NZQzM3S1NBRjJQTEJMQ05PIiwidHlwZSI6InVzZXIiLCJ2ZXJzaW9uIjoyfX0.FUEg-P5f4axZta_kci_B0VrqdEHR2_et95ZaetVVhABVMiH2RSbnAZINQpqhKt7j_WBV6NLh2sEyTQRwAe0XCA",
		"operator": "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiJFM1pQS1hVQkVWTlJTQU5FMlZSSFNMUEtYVUk0VFI0NEozSk9DT1hST0ZWMkhUSkk1VlVBIiwiaWF0IjoxNzU2MjM1MTUwLCJpc3MiOiJPQ0xCWEhIQ0NWV1pCWjVKS1I3N1I3UE1QTjRBTlROT0RKNERBVDdUVUVYSUJCVktCTURTTlE3QiIsIm5hbWUiOiJPIiwic3ViIjoiT0NMQlhISENDVldaQlo1SktSNzdSN1BNUE40QU5UTk9ESjREQVQ3VFVFWElCQlZLQk1EU05RN0IiLCJuYXRzIjp7InN5c3RlbV9hY2NvdW50IjoiQUIyWFpBRUFYM0FBSDRURFNGVFNURkFTQTVHRUpJTFhIVUtXTUFQNkRHTk1TUTcyRUhBREhOUVciLCJ0eXBlIjoib3BlcmF0b3IiLCJ2ZXJzaW9uIjoyfX0.-gyahNWSzvnZJ94SoECf3ru7KXrxxiOpbN-m4aku7jVxkCEf5l9a5Lx5lKNSvsGNrZlrQs0DNvI9BoP3rKm8Ag",
		"system_account": "AB2XZAEAX3AAH4TDSFTSTFASA5GEJILXHUKWMAP6DGNMSQ72EHADHNQW",
		"resolver": {
			"type": "MEMORY"
		},
		"resolver_preload": {
			"AB2XZAEAX3AAH4TDSFTSTFASA5GEJILXHUKWMAP6DGNMSQ72EHADHNQW": "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiJXRE1ERU1FMktWU1FOWkZERU9DN1VGNkRMQ0M0RjZXVFZLSEg1RlY2TVQ3N1dZS0VTV0tRIiwiaWF0IjoxNzU2MjM1MTUwLCJpc3MiOiJPQ0xCWEhIQ0NWV1pCWjVKS1I3N1I3UE1QTjRBTlROT0RKNERBVDdUVUVYSUJCVktCTURTTlE3QiIsIm5hbWUiOiJTWVMiLCJzdWIiOiJBQjJYWkFFQVgzQUFINFREU0ZUU1RGQVNBNUdFSklMWEhVS1dNQVA2REdOTVNRNzJFSEFESE5RVyIsIm5hdHMiOnsibGltaXRzIjp7InN1YnMiOi0xLCJkYXRhIjotMSwicGF5bG9hZCI6LTEsImltcG9ydHMiOi0xLCJleHBvcnRzIjotMSwid2lsZGNhcmRzIjp0cnVlLCJjb25uIjotMSwibGVhZiI6LTF9LCJkZWZhdWx0X3Blcm1pc3Npb25zIjp7InB1YiI6e30sInN1YiI6e319LCJhdXRob3JpemF0aW9uIjp7fSwidHlwZSI6ImFjY291bnQiLCJ2ZXJzaW9uIjoyfX0.angl8gTaixZiFaPl4xY1sb6p4f32WEkWhi4z7co7WqqWymSG8Y_ovFej-TsJ4YHxjtWk6WA82lEHWcOUNj9WDA",
			"ABZXH5QFKLB757UMLKGF6FLNQCVJAB2T5AVMOTLD6AN5GMW7CNB5EJJY": "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiJaMkZITElXSktYNllFR05CTFJSUDJZRlhQWlpVM0tGN1NSVkI1NVBHNFhQT0w2QlA2TUZRIiwiaWF0IjoxNzU2MjM1MTUwLCJpc3MiOiJPQ0xCWEhIQ0NWV1pCWjVKS1I3N1I3UE1QTjRBTlROT0RKNERBVDdUVUVYSUJCVktCTURTTlE3QiIsIm5hbWUiOiJBIiwic3ViIjoiQUJaWEg1UUZLTEI3NTdVTUxLR0Y2RkxOUUNWSkFCMlQ1QVZNT1RMRDZBTjVHTVc3Q05CNUVKSlkiLCJuYXRzIjp7ImxpbWl0cyI6eyJzdWJzIjotMSwiZGF0YSI6LTEsInBheWxvYWQiOi0xLCJpbXBvcnRzIjotMSwiZXhwb3J0cyI6LTEsIndpbGRjYXJkcyI6dHJ1ZSwiY29ubiI6LTEsImxlYWYiOi0xfSwiZGVmYXVsdF9wZXJtaXNzaW9ucyI6eyJwdWIiOnt9LCJzdWIiOnt9fSwiYXV0aG9yaXphdGlvbiI6e30sInR5cGUiOiJhY2NvdW50IiwidmVyc2lvbiI6Mn19.f3Z5YXmsJSzw09fPKiszL3dGTmjjplaAKcUwIZFpsVl9vmFCMR-JC3JX3f3khLZHXxT23QqP-CZL1qrp1XuwDg",
			"ADB77I2PCJSREARW64XVPQT3T55NPRVRX37UBG7CYC37KSAF2PLBLCNO": "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiJJQVRXQzZZNlg3VlZEUkxKTUlFREpER1hQSEZaWDRQNTQyTFhCRjNUTzZMTzY1WDNJNlFBIiwiaWF0IjoxNzU2MjM1MTUwLCJpc3MiOiJPQ0xCWEhIQ0NWV1pCWjVKS1I3N1I3UE1QTjRBTlROT0RKNERBVDdUVUVYSUJCVktCTURTTlE3QiIsIm5hbWUiOiJDIiwic3ViIjoiQURCNzdJMlBDSlNSRUFSVzY0WFZQUVQzVDU1TlBSVlJYMzdVQkc3Q1lDMzdLU0FGMlBMQkxDTk8iLCJuYXRzIjp7ImxpbWl0cyI6eyJzdWJzIjotMSwiZGF0YSI6LTEsInBheWxvYWQiOi0xLCJpbXBvcnRzIjotMSwiZXhwb3J0cyI6LTEsIndpbGRjYXJkcyI6dHJ1ZSwiY29ubiI6LTEsImxlYWYiOi0xfSwic2lnbmluZ19rZXlzIjpbeyJraW5kIjoidXNlcl9zY29wZSIsImtleSI6IkFCRDU3R1M1V1VMV1ROVkFXNkZVSFRLTFE3VUlZSUFZUEtIUUo2UEpGQ1lSWTdNUlg3N1pFRERGIiwicm9sZSI6InNlbnRpbmVsIiwidGVtcGxhdGUiOnsicHViIjp7ImRlbnkiOlsiXHUwMDNlIl19LCJzdWIiOnsiZGVueSI6WyJcdTAwM2UiXX0sInN1YnMiOi0xLCJkYXRhIjotMSwicGF5bG9hZCI6LTEsImJlYXJlcl90b2tlbiI6dHJ1ZX0sImRlc2NyaXB0aW9uIjoiIn1dLCJkZWZhdWx0X3Blcm1pc3Npb25zIjp7InB1YiI6e30sInN1YiI6e319LCJhdXRob3JpemF0aW9uIjp7ImF1dGhfdXNlcnMiOlsiVUREWFRCQkdISEc0TDZCTFBTQVY1VlhSNVBFRlFaV1o0U0pRWEZUU01PV1RWSEJWVUdRNE9XRDUiXSwiYWxsb3dlZF9hY2NvdW50cyI6WyJBQlpYSDVRRktMQjc1N1VNTEtHRjZGTE5RQ1ZKQUIyVDVBVk1PVExENkFONUdNVzdDTkI1RUpKWSJdfSwidHlwZSI6ImFjY291bnQiLCJ2ZXJzaW9uIjoyfX0.aJNjr998iXsmcYDxWq_1HOQARQkyAa5UzcHTXQMu10A8BNfQhMGSKh-6NyQ9D2JVsilyrcAxdV_dsPQdSw5MBQ"
		}
	}`

	var conf ResolverConf
	err := json.Unmarshal([]byte(jsonData), &conf)
	require.NoError(t, err)

	// Verify DefaultSentinel
	require.NotEmpty(t, conf.DefaultSentinel)
	require.Equal(t, "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiIyR1FRQjdTUlJOUklRNVIyRUxTVklSWFlRQzZBQTVRV1IyUlhRTFlHUklINVZINE1LVENBIiwiaWF0IjoxNzU2MjM1MTUwLCJpc3MiOiJBQkQ1N0dTNVdVTFdUTlZBVzZGVUhUS0xRN1VJWUlBWVBLSFFKNlBKRkNZUlk3TVJYNzdaRURERiIsIm5hbWUiOiJzZW50aW5lbCIsInN1YiI6IlVDNzNLMjVLRjZPWlpRVVhIQlZSS0hMUklRRlU1NEdJUUk0TlFYMlNBSFdBVU9CSDY3UTRWSTVZIiwibmF0cyI6eyJwdWIiOnt9LCJzdWIiOnt9LCJpc3N1ZXJfYWNjb3VudCI6IkFEQjc3STJQQ0pTUkVBUlc2NFhWUFFUM1Q1NU5QUlZSWDM3VUJHN0NZQzM3S1NBRjJQTEJMQ05PIiwidHlwZSI6InVzZXIiLCJ2ZXJzaW9uIjoyfX0.FUEg-P5f4axZta_kci_B0VrqdEHR2_et95ZaetVVhABVMiH2RSbnAZINQpqhKt7j_WBV6NLh2sEyTQRwAe0XCA", conf.DefaultSentinel)

	// Verify Operator JWT
	require.NotEmpty(t, conf.Operator)
	require.Equal(t, "eyJ0eXAiOiJKV1QiLCJhbGciOiJlZDI1NTE5LW5rZXkifQ.eyJqdGkiOiJFM1pQS1hVQkVWTlJTQU5FMlZSSFNMUEtYVUk0VFI0NEozSk9DT1hST0ZWMkhUSkk1VlVBIiwiaWF0IjoxNzU2MjM1MTUwLCJpc3MiOiJPQ0xCWEhIQ0NWV1pCWjVKS1I3N1I3UE1QTjRBTlROT0RKNERBVDdUVUVYSUJCVktCTURTTlE3QiIsIm5hbWUiOiJPIiwic3ViIjoiT0NMQlhISENDVldaQlo1SktSNzdSN1BNUE40QU5UTk9ESjREQVQ3VFVFWElCQlZLQk1EU05RN0IiLCJuYXRzIjp7InN5c3RlbV9hY2NvdW50IjoiQUIyWFpBRUFYM0FBSDRURFNGVFNURkFTQTVHRUpJTFhIVUtXTUFQNkRHTk1TUTcyRUhBREhOUVciLCJ0eXBlIjoib3BlcmF0b3IiLCJ2ZXJzaW9uIjoyfX0.-gyahNWSzvnZJ94SoECf3ru7KXrxxiOpbN-m4aku7jVxkCEf5l9a5Lx5lKNSvsGNrZlrQs0DNvI9BoP3rKm8Ag", conf.Operator)

	// Verify SystemAccount
	require.Equal(t, "AB2XZAEAX3AAH4TDSFTSTFASA5GEJILXHUKWMAP6DGNMSQ72EHADHNQW", conf.SystemAccount)

	// Verify Resolver
	require.NotNil(t, conf.Resolver)
	require.Equal(t, ResolverType("MEMORY"), conf.Resolver.Type)

	// Verify Preload map
	require.NotNil(t, conf.Preload)
	require.Len(t, conf.Preload, 3)

	// Check specific preloaded accounts
	sysJWT, ok := conf.Preload["AB2XZAEAX3AAH4TDSFTSTFASA5GEJILXHUKWMAP6DGNMSQ72EHADHNQW"]
	require.True(t, ok)
	require.NotEmpty(t, sysJWT)

	accA, ok := conf.Preload["ABZXH5QFKLB757UMLKGF6FLNQCVJAB2T5AVMOTLD6AN5GMW7CNB5EJJY"]
	require.True(t, ok)
	require.NotEmpty(t, accA)

	accC, ok := conf.Preload["ADB77I2PCJSREARW64XVPQT3T55NPRVRX37UBG7CYC37KSAF2PLBLCNO"]
	require.True(t, ok)
	require.NotEmpty(t, accC)
}
