// testing_helpers.go — KAI-235 test-only exports on Server.
//
// These methods expose internal state that isolation tests need to:
//   - enumerate registered routes (TestIsolation_AllRoutesCovered)
//   - inject broken handlers (TestNegative_BrokenIsolation_Detected)
//
// They are NOT restricted to _test.go because the isolation package imports
// the apiserver package directly (it is not in the same package). If this
// becomes a concern, move them behind a build tag; for now the API surface is
// narrow and safe.
package apiserver

import "net/http"

// RegisteredRoutes returns the set of URL paths that are wired into the
// server's mux. The list is built from the static connectServices table and
// the additional routes mounted in New(). It does NOT include dynamic handler
// paths registered via MuxHandle after construction.
//
// Used by KAI-235 TestIsolation_AllRoutesCovered to enforce 100% isolation
// test coverage of every authenticated route.
func (s *Server) RegisteredRoutes() []string {
	paths := make([]string, 0, len(connectServices)*5+4)

	// Connect-Go service paths.
	for _, svc := range connectServices {
		for _, m := range svc.methods {
			paths = append(paths, ServicePath(svc.service, m))
		}
	}

	// Additional plain-HTTP routes mounted in New().
	paths = append(paths,
		"/healthz",
		"/readyz",
		"/metrics",
	)
	if s.cfg.StreamsService != nil {
		paths = append(paths, "/api/v1/streams/request")
	}
	if s.cfg.StreamsIssuer != nil {
		paths = append(paths, "/.well-known/jwks.json")
	}

	return paths
}

// MuxHandle registers handler at path on the server's mux. It is the test
// escape hatch for the negative isolation test: production code should not
// call this after New() returns.
func (s *Server) MuxHandle(path string, handler http.Handler) {
	s.mux.Handle(path, handler)
}

// RegisterRoute adds a RouteAuthorization entry to the server's route table.
// This is used by the negative isolation test to give the permission
// middleware a policy entry for a sandboxed broken handler.
func (s *Server) RegisterRoute(path string, auth RouteAuthorization) {
	s.routes[path] = auth
}

// BuildConnectChainPublic exposes buildConnectChain so tests can wrap custom
// handlers in the same auth + permission + audit middleware chain that
// production handlers use.
func (s *Server) BuildConnectChainPublic() Middleware {
	return s.buildConnectChain()
}
