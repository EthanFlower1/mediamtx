package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ZitadelClient is the interface this package requires from the Zitadel
// adapter. It is satisfied by *zitadel.Adapter but defined here to avoid
// importing the zitadel package (seam #3). The caller passes in a concrete
// adapter.
type ZitadelClient interface {
	// DoManagementAPI executes a JSON request against Zitadel's management API.
	// orgID scopes the request to a specific org via the x-zitadel-orgid header.
	DoManagementAPI(ctx context.Context, method, path, orgID string, reqBody, respBody any) error
}

// Run executes the full bootstrap sequence. It is idempotent.
func Run(ctx context.Context, client ZitadelClient, cfg Config) (*Result, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	result := &Result{
		Apps: make(map[string]string, len(cfg.Apps)),
	}

	// Step 1: Create platform org (or recover existing).
	orgID, err := ensurePlatformOrg(ctx, client, cfg.PlatformOrgName)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: create platform org: %w", err)
	}
	result.PlatformOrgID = orgID

	// Step 2: Create service account.
	saID, err := ensureServiceAccount(ctx, client, orgID, cfg.ServiceAccountName)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: create service account: %w", err)
	}
	result.ServiceAccountID = saID

	// Step 3: Generate JWT key for the service account.
	keyJSON, err := generateServiceAccountKey(ctx, client, orgID, saID)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: generate SA key: %w", err)
	}
	result.ServiceAccountKeyJSON = keyJSON

	// Step 4: Grant org-manager + user-manager roles.
	if err := ensureRoles(ctx, client, orgID, saID); err != nil {
		return nil, fmt.Errorf("bootstrap: grant roles: %w", err)
	}

	// Step 5: Register OIDC applications.
	for _, app := range cfg.Apps {
		clientID, err := ensureOIDCApp(ctx, client, orgID, app)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: create OIDC app %q: %w", app.Name, err)
		}
		result.Apps[app.Name] = clientID
	}

	return result, nil
}

func ensurePlatformOrg(ctx context.Context, client ZitadelClient, name string) (string, error) {
	req := map[string]any{"name": name}
	var resp struct {
		ID string `json:"id"`
	}
	err := client.DoManagementAPI(ctx, http.MethodPost, "/management/v1/orgs", "", req, &resp)
	if err != nil {
		// Check for 409 conflict (org already exists).
		if isConflict(err) {
			return findOrgByName(ctx, client, name)
		}
		return "", err
	}
	return resp.ID, nil
}

func ensureServiceAccount(ctx context.Context, client ZitadelClient, orgID, username string) (string, error) {
	req := map[string]any{
		"userName":    username,
		"name":        username,
		"description": "Kaivue platform service account (KAI-221)",
		"accessTokenType": 1, // JWT
	}
	var resp struct {
		UserID string `json:"userId"`
	}
	err := client.DoManagementAPI(ctx, http.MethodPost, "/management/v1/users/machine", orgID, req, &resp)
	if err != nil {
		if isConflict(err) {
			return findMachineUserByName(ctx, client, orgID, username)
		}
		return "", err
	}
	return resp.UserID, nil
}

func generateServiceAccountKey(ctx context.Context, client ZitadelClient, orgID, userID string) ([]byte, error) {
	req := map[string]any{
		"type": 1, // KEY_TYPE_JSON
	}
	var resp struct {
		KeyDetails json.RawMessage `json:"keyDetails"`
	}
	path := fmt.Sprintf("/management/v1/users/%s/keys", userID)
	if err := client.DoManagementAPI(ctx, http.MethodPost, path, orgID, req, &resp); err != nil {
		return nil, err
	}
	return resp.KeyDetails, nil
}

func ensureRoles(ctx context.Context, client ZitadelClient, orgID, userID string) error {
	roles := []string{"ORG_OWNER"}
	for _, role := range roles {
		req := map[string]any{
			"userId": userID,
			"roleKeys": []string{role},
		}
		path := fmt.Sprintf("/management/v1/orgs/me/members")
		err := client.DoManagementAPI(ctx, http.MethodPost, path, orgID, req, nil)
		if err != nil && !isConflict(err) {
			return fmt.Errorf("grant role %s: %w", role, err)
		}
	}
	return nil
}

func ensureOIDCApp(ctx context.Context, client ZitadelClient, orgID string, app OIDCApp) (string, error) {
	// First create a project (or find existing).
	projectID, err := ensureProject(ctx, client, orgID, "kaivue")
	if err != nil {
		return "", fmt.Errorf("ensure project: %w", err)
	}

	// Determine OIDC grant and response types.
	grantTypes := []string{"OIDC_GRANT_TYPE_AUTHORIZATION_CODE"}
	responseTypes := []string{"OIDC_RESPONSE_TYPE_CODE"}
	appType := "OIDC_APP_TYPE_WEB"
	authMethod := "OIDC_AUTH_METHOD_TYPE_BASIC"

	switch app.Type {
	case OIDCAppNative:
		appType = "OIDC_APP_TYPE_NATIVE"
		authMethod = "OIDC_AUTH_METHOD_TYPE_NONE"
	case OIDCAppSPA:
		appType = "OIDC_APP_TYPE_USER_AGENT"
		authMethod = "OIDC_AUTH_METHOD_TYPE_NONE"
	}

	req := map[string]any{
		"name":                       app.Name,
		"redirectUris":               app.RedirectURIs,
		"postLogoutRedirectUris":     app.PostLogoutRedirectURIs,
		"responseTypes":              responseTypes,
		"grantTypes":                 grantTypes,
		"appType":                    appType,
		"authMethodType":             authMethod,
		"devMode":                    true,
		"accessTokenType":            1, // JWT
		"accessTokenRoleAssertion":   true,
		"idTokenRoleAssertion":       true,
		"idTokenUserinfoAssertion":   true,
	}
	var resp struct {
		AppID    string `json:"appId"`
		ClientID string `json:"clientId"`
	}
	path := fmt.Sprintf("/management/v1/projects/%s/apps/oidc", projectID)
	err = client.DoManagementAPI(ctx, http.MethodPost, path, orgID, req, &resp)
	if err != nil {
		if isConflict(err) {
			return findOIDCAppByName(ctx, client, orgID, projectID, app.Name)
		}
		return "", err
	}
	return resp.ClientID, nil
}

func ensureProject(ctx context.Context, client ZitadelClient, orgID, name string) (string, error) {
	req := map[string]any{
		"name":                 name,
		"projectRoleAssertion": true,
		"projectRoleCheck":     true,
	}
	var resp struct {
		ID string `json:"id"`
	}
	err := client.DoManagementAPI(ctx, http.MethodPost, "/management/v1/projects", orgID, req, &resp)
	if err != nil {
		if isConflict(err) {
			return findProjectByName(ctx, client, orgID, name)
		}
		return "", err
	}
	return resp.ID, nil
}

// --- Search helpers for idempotent recovery ---

func findOrgByName(ctx context.Context, client ZitadelClient, name string) (string, error) {
	req := map[string]any{
		"queries": []map[string]any{
			{"nameQuery": map[string]any{"name": name, "method": "TEXT_QUERY_METHOD_EQUALS"}},
		},
	}
	var resp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := client.DoManagementAPI(ctx, http.MethodPost, "/management/v1/orgs/_search", "", req, &resp); err != nil {
		return "", err
	}
	for _, o := range resp.Result {
		if o.Name == name {
			return o.ID, nil
		}
	}
	return "", fmt.Errorf("org %q: conflict but not found by name search", name)
}

func findMachineUserByName(ctx context.Context, client ZitadelClient, orgID, username string) (string, error) {
	req := map[string]any{
		"queries": []map[string]any{
			{"userNameQuery": map[string]any{"userName": username, "method": "TEXT_QUERY_METHOD_EQUALS"}},
		},
	}
	var resp struct {
		Result []struct {
			ID       string `json:"id"`
			UserName string `json:"userName"`
		} `json:"result"`
	}
	if err := client.DoManagementAPI(ctx, http.MethodPost, "/management/v1/users/_search", orgID, req, &resp); err != nil {
		return "", err
	}
	for _, u := range resp.Result {
		if u.UserName == username {
			return u.ID, nil
		}
	}
	return "", fmt.Errorf("machine user %q: conflict but not found", username)
}

func findProjectByName(ctx context.Context, client ZitadelClient, orgID, name string) (string, error) {
	req := map[string]any{
		"queries": []map[string]any{
			{"nameQuery": map[string]any{"name": name, "method": "TEXT_QUERY_METHOD_EQUALS"}},
		},
	}
	var resp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"result"`
	}
	if err := client.DoManagementAPI(ctx, http.MethodPost, "/management/v1/projects/_search", orgID, req, &resp); err != nil {
		return "", err
	}
	for _, p := range resp.Result {
		if p.Name == name {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf("project %q: conflict but not found", name)
}

func findOIDCAppByName(ctx context.Context, client ZitadelClient, orgID, projectID, name string) (string, error) {
	var resp struct {
		Result []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			OIDCConfig struct {
				ClientID string `json:"clientId"`
			} `json:"oidcConfig"`
		} `json:"result"`
	}
	path := fmt.Sprintf("/management/v1/projects/%s/apps/_search", projectID)
	if err := client.DoManagementAPI(ctx, http.MethodPost, path, orgID, map[string]any{}, &resp); err != nil {
		return "", err
	}
	for _, a := range resp.Result {
		if a.Name == name {
			return a.OIDCConfig.ClientID, nil
		}
	}
	return "", fmt.Errorf("OIDC app %q: conflict but not found", name)
}

// isConflict checks if an error represents an HTTP 409 Conflict.
// This is a duck-type check that looks for a StatusCode method.
type statusCoder interface {
	StatusCode() int
}

func isConflict(err error) bool {
	var sc statusCoder
	if ok := asStatusCoder(err, &sc); ok {
		return sc.StatusCode() == http.StatusConflict
	}
	return false
}

func asStatusCoder(err error, target *statusCoder) bool {
	type hasStatus interface {
		StatusCode() int
	}
	for err != nil {
		if hs, ok := err.(hasStatus); ok {
			*target = hs
			return true
		}
		err = unwrapErr(err)
	}
	return false
}

func unwrapErr(err error) error {
	type wrapper interface {
		Unwrap() error
	}
	if w, ok := err.(wrapper); ok {
		return w.Unwrap()
	}
	return nil
}
