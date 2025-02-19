package nst

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	authb "github.com/synadia-io/jwt-auth-builder.go"
)

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

func ClientInfo(t testing.TB, nc *nats.Conn) UserInfo {
	r, err := nc.Request(UserInfoSubj, nil, time.Second*2)
	require.NoError(t, err)
	require.NotNil(t, r)
	var info UserInfo
	require.NoError(t, json.Unmarshal(r.Data, &info))
	return info
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

//func (ts *EmbeddedNatsServer) ConnectAccount(account string, user string, bearer bool) (*nats.Conn, error) {
//	u := ts.Resolver.Identities.CreateUser(account, user, bearer)
//	return ts.MaybeConnect(u.ConnectOptions())
//}

//func NewResolverConfig(t *testing.T, dir string) *ResolverConf {
//	v := &ResolverConf{
//		t:          t,
//		Listen:     "127.0.0.1:-1",
//		Identities: NewIdentities(t),
//		Resolver: Resolver{
//			Type:           "full",
//			Dir:            filepath.Join(dir, "jwts"),
//			UpdateInterval: time.Minute,
//			Timeout:        time.Millisecond * 1900,
//		},
//	}
//
//	v.Operator = v.Identities.Operator.Token
//	v.SystemAccount = v.Identities.System.PublicKey()
//
//	for _, a := range v.Identities.Accounts {
//		v.addAccount(a)
//	}
//
//	require.NoError(t, os.MkdirAll(v.Resolver.Dir, 0o777))
//
//	return v
//}
//
//func (r *ResolverConf) Store(parentDir string) string {
//	f, err := os.CreateTemp(parentDir, "server.conf")
//	require.NoError(r.t, err)
//	defer func() {
//		_ = f.Close()
//	}()
//
//	d, err := json.MarshalIndent(r, "", " ")
//	require.NoError(r.t, err)
//
//	_, err = f.Write(d)
//	require.NoError(r.t, err)
//
//	return f.Name()
//}
//
//func (r *ResolverConf) NewAccount(name string) *TokenKP {
//	a := r.Identities.AddAccount(name)
//	r.addAccount(a)
//	return a
//}
//
//func (r *ResolverConf) addAccount(acc *TokenKP) {
//	if r.Preload == nil {
//		r.Preload = make(map[string]string)
//	}
//	r.Preload[acc.PublicKey()] = acc.Token
//}
//
//type TokenKP struct {
//	Token  string
//	KP     nkeys.KeyPair
//	bearer bool
//}
//
//func (kp *TokenKP) ConnectOptions() nats.Option {
//	return func(options *nats.Options) error {
//		options.UserJWT = func() (string, error) {
//			return kp.Token, nil
//		}
//		options.SignatureCB = func(nonce []byte) ([]byte, error) {
//			if kp.bearer {
//				return nil, nil
//			}
//			return kp.KP.Sign(nonce)
//		}
//		return nil
//	}
//}
//
//func (t *TokenKP) PublicKey() string {
//	pk, _ := t.KP.PublicKey()
//	return pk
//}
