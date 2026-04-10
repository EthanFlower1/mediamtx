// Package idptest provides integration test helpers for the Zitadel
// adapter's identity-provider configuration surface (KAI-137).
//
// All tests in this package are guarded by the "integration" build tag
// so they are skipped during normal `go test ./...` runs. To execute
// them you need the mock-IdP Docker Compose stack running:
//
//	docker compose -f test/idp/docker-compose.yml up -d
//	go test -tags integration -v ./internal/shared/auth/zitadel/idptest/...
//	docker compose -f test/idp/docker-compose.yml down -v
//
// The compose stack provides:
//   - Mock OIDC server at http://localhost:9090 (navikt/mock-oauth2-server)
//   - Mock SAML IdP at http://localhost:8080 (kristophjunge/test-saml-idp)
//   - OpenLDAP at ldap://localhost:389 pre-seeded with test users/groups
package idptest
