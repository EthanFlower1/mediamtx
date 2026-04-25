package broker

// Authenticator validates API keys against the broker store and returns the
// associated tenant ID. Its Authenticate method matches the signature
// expected by connect.BrokerConfig.Authenticate.
type Authenticator struct {
	store *Store
}

// NewAuthenticator creates an Authenticator backed by the given Store.
func NewAuthenticator(store *Store) *Authenticator {
	return &Authenticator{store: store}
}

// Authenticate checks the provided token (a plain-text API key) against the
// store. If valid, it returns the tenant ID and true. On any error (invalid
// key, DB failure, etc.) it returns ("", false).
func (a *Authenticator) Authenticate(token string) (tenantID string, ok bool) {
	tid, err := a.store.ValidateAPIKey(token)
	if err != nil {
		return "", false
	}
	return tid, true
}
