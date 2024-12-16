package nst

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/nats-io/jwt/v2"
	"github.com/stretchr/testify/require"
	"github.com/synadia-io/jwt-auth-builder.go"
)

func ResolverFromAuth(t *testing.T, operator authb.Operator) *ResolverConf {
	var config ResolverConf
	config.Resolver.Type = "mem"

	require.NotNil(t, operator)
	config.Operator = operator.JWT()
	sys, err := operator.SystemAccount()
	require.NoError(t, err)
	if sys != nil {
		config.SystemAccount = sys.Subject()
	}
	accounts := operator.Accounts().List()
	config.Preload = make(map[string]string)
	for _, a := range accounts {
		config.Preload[a.Subject()] = a.JWT()
	}
	return &config
}

// Conf rudimentary struct representing a configuration, missing most :)
type Conf struct {
	Authorization Authorization `json:"authorization,omitempty"`
}

// Marshal serializes a Conf into JSON. This is necessary because
// some permissions setups will have subject wildcards which will
// serialize incorrectly as JSON.
func (c Conf) Marshal(t *testing.T) []byte {
	buffer := &bytes.Buffer{}
	encoder := json.NewEncoder(buffer)
	encoder.SetEscapeHTML(false)
	err := encoder.Encode(c)
	require.NoError(t, err)
	return buffer.Bytes()
}

// Authorization block
type Authorization struct {
	Users Users `json:"users,omitempty"`
}

// Users block
type Users []User

func (u *Users) Add(user ...User) {
	for _, v := range user {
		*u = append(*u, v)
	}
}

// User represents Username/Password/Token/Permissions
type User struct {
	User        string      `json:"user,omitempty"`
	Password    string      `json:"password,omitempty"`
	Token       string      `json:"token,omitempty"`
	Permissions Permissions `json:"permissions,omitempty"`
}

// Permissions block
type Permissions struct {
	Publish        AllowDeny `json:"publish,omitempty"`
	Subscribe      AllowDeny `json:"subscribe,omitempty"`
	AllowResponses bool      `json:"allow_responses,omitempty"`
}

// AllowDeny block
type AllowDeny struct {
	Allow jwt.StringList `json:"allow,omitempty"`
	Deny  jwt.StringList `json:"deny,omitempty"`
}

// ResolverConf a Conf using delegated authentication
type ResolverConf struct {
	Operator      string            `json:"operator,omitempty"`
	SystemAccount string            `json:"system_account,omitempty"`
	Resolver      Resolver          `json:"resolver,omitempty"`
	Preload       map[string]string `json:"resolver_preload,omitempty"`
}

type Resolver struct {
	Type           string        `json:"type,omitempty"`
	Dir            string        `json:"dir,omitempty"`
	AllowDelete    bool          `json:"allow_delete,omitempty"`
	UpdateInterval time.Duration `json:"interval,omitempty"`
	Timeout        time.Duration `json:"timeout,omitempty"`
}

func (r *ResolverConf) Marshal(t *testing.T) []byte {
	d, err := json.MarshalIndent(r, "", "  ")
	require.NoError(t, err)
	return d
}

func (r *Resolver) MarshalJSON() ([]byte, error) {
	type RC struct {
		Resolver
		SInterval string `json:"interval"`
		STimeout  string `json:"timeout"`
	}

	rc := RC{Resolver: *r}
	rc.SInterval = r.UpdateInterval.String()
	rc.STimeout = r.Timeout.String()

	return json.Marshal(rc)
}
