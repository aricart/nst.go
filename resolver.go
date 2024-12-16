package nst

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
