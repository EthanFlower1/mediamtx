package kaivue

import "net/http"

// AuthProvider applies authentication to outgoing HTTP requests.
type AuthProvider interface {
	Apply(req *http.Request)
}

// APIKeyAuth authenticates via the X-API-Key header.
type APIKeyAuth struct {
	Key string
}

// Apply sets the X-API-Key header.
func (a *APIKeyAuth) Apply(req *http.Request) {
	req.Header.Set("X-API-Key", a.Key)
}

// OAuthAuth authenticates via a Bearer token in the Authorization header.
type OAuthAuth struct {
	AccessToken string
}

// Apply sets the Authorization header.
func (a *OAuthAuth) Apply(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+a.AccessToken)
}
