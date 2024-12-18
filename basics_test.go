package nst

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nkeys"

	authb "github.com/synadia-io/jwt-auth-builder.go"
	"github.com/synadia-io/jwt-auth-builder.go/providers/nsc"

	"github.com/nats-io/nats.go/jetstream"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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
	ns := NewNatsServer(s.T(), nil)
	defer ns.Shutdown()

	nc := ns.Connect()
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
	ns := NewNatsServer(s.T(), nil)
	nc := ns.Connect()
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
	conf.Authorization.Users.Add(u)

	fn := td.WriteFile("server.conf", conf.Marshal(s.T()))

	ns := NewNatsServer(s.T(), &natsserver.Options{
		ConfigFile: fn,
		Debug:      true,
		Trace:      true,
		NoLog:      false,
	})
	defer ns.Shutdown()

	nc := ns.ConnectWithOptions(nats.Options{User: "a", Password: "b"})
	defer nc.Close()

	info := ClientInfo(s.T(), nc)
	s.Len(info.Data.Permissions.Pub.Allow, 2)
	s.Contains(info.Data.Permissions.Pub.Allow, "echo")
	s.Contains(info.Data.Permissions.Pub.Allow, UserInfoSubj)
	s.Len(info.Data.Permissions.Sub.Allow, 1)
	s.Contains(info.Data.Permissions.Sub.Allow, "_INBOX.>")
	s.False(info.Data.Permissions.AllowResponses)
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

	ns := NewNatsServer(s.T(), &natsserver.Options{
		ConfigFile: fn,
		Debug:      true,
		Trace:      true,
		NoLog:      false,
	})
	defer ns.Shutdown()

	nc := ns.ConnectWithOptions(nats.Options{User: "auth", Password: "pwd"})
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

	ns := NewNatsServer(s.T(), &natsserver.Options{
		ConfigFile: fn,
		Debug:      true,
		Trace:      true,
		NoLog:      false,
	})
	defer ns.Shutdown()

	sa := ns.Server.SystemAccount()
	s.NotNil(sa)
	s.Equal("SYS", sa.Name)
}

func (s *BasicTestSuite) TestServerConfigJetStream() {
	td := NewTestDir(s.T(), "", "nst-test")
	defer td.Cleanup()

	opts := DefaultNatsServerWithJetStreamOptions(td.Dir)
	opts.Debug = true
	opts.Trace = true

	ns := NewNatsServer(s.T(), opts)
	defer ns.Shutdown()

	nc := ns.Connect()
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

	ns := NewNatsServer(t, &natsserver.Options{
		ConfigFile: td.WriteFile("server.conf", resolver.Marshal(t)),
		Debug:      true,
		Trace:      true,
		NoLog:      false,
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
