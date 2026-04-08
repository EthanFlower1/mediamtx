// TODO(KAI-310): replace with generated connectrpc code once buf is wired.
//
// This file registers placeholder handlers for every Connect-Go service in
// internal/shared/proto/v1. Each handler returns a Connect-style
// Unimplemented error. The ROUTING is real — the paths match the canonical
// "/<proto-package>.<service>/<method>" shape Connect-Go uses at wire level,
// so the middleware chain exercised by tests is the same chain that will run
// in production once KAI-310 lands.
package apiserver

import (
	"errors"
	"net/http"
)

// ServicePath returns the canonical Connect path for a (service, method)
// pair. Exported so tests and sibling tickets (KAI-224, KAI-227) can build
// the same paths without stringly-typed drift.
func ServicePath(service, method string) string {
	return "/kaivue.v1." + service + "/" + method
}

// connectServices is the list of (service name, method) pairs this server
// hosts. Keep it in alphabetical order by service so code review diffs are
// friendly.
var connectServices = []struct {
	service string
	methods []string
}{
	{"AuthService", []string{
		"Login",
		"Refresh",
		"RevokeSession",
		"BeginSSOFlow",
		"CompleteSSOFlow",
	}},
	{"CamerasService", []string{
		"CreateCamera",
		"GetCamera",
		"UpdateCamera",
		"DeleteCamera",
		"ListCameras",
	}},
	{"DirectoryIngestService", []string{
		"StreamCameraState",     // server-stream Recorder → Directory
		"PublishSegmentIndex",
		"PublishAIEvents",
	}},
	{"RecorderControlService", []string{
		"StreamAssignments", // server-stream Directory → Recorder
		"Heartbeat",
	}},
	{"StreamsService", []string{
		"MintStreamURL",
		"RevokeStream",
	}},
	// Sibling-owned services: the apiserver reserves the path but
	// delegates to the owning team's handler (KAI-224 cross-tenant,
	// KAI-227 tenant provisioning). Until those land we stub them too.
	{"CrossTenantService", []string{
		"ListAccessibleCustomers",
		"MintDelegatedToken",
	}},
	{"TenantProvisioningService", []string{
		"CreateTenant",
		"SuspendTenant",
	}},
}

// unimplementedHandler is the per-method placeholder. It returns a Connect
// Unimplemented error so clients see the same wire-format response they
// will see once generated handlers replace these stubs.
func unimplementedHandler(service, method string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, NewError(CodeUnimplemented, errors.New("TODO(KAI-310): "+service+"."+method+" not yet generated")))
	})
}

// defaultRouteAuthorizations is the hard-coded Casbin (resource, action)
// mapping for the stub services. Replacing this with a reflection-based
// scan of the generated FileDescriptors is part of KAI-310.
//
// The goal right now is not to have a perfect ACL, it is to give the
// permission middleware an entry per path so authorised-vs-unauthorised
// integration tests can exercise the 2xx/403 flow.
func defaultRouteAuthorizations() map[string]RouteAuthorization {
	return map[string]RouteAuthorization{
		ServicePath("CamerasService", "CreateCamera"): {ResourceType: "cameras", Action: "create"},
		ServicePath("CamerasService", "GetCamera"):    {ResourceType: "cameras", Action: "read"},
		ServicePath("CamerasService", "UpdateCamera"): {ResourceType: "cameras", Action: "update"},
		ServicePath("CamerasService", "DeleteCamera"): {ResourceType: "cameras", Action: "delete"},
		ServicePath("CamerasService", "ListCameras"):  {ResourceType: "cameras", Action: "read"},

		ServicePath("StreamsService", "MintStreamURL"): {ResourceType: "streams", Action: "mint"},
		ServicePath("StreamsService", "RevokeStream"):  {ResourceType: "streams", Action: "revoke"},

		ServicePath("RecorderControlService", "StreamAssignments"): {ResourceType: "recorders", Action: "control"},
		ServicePath("RecorderControlService", "Heartbeat"):         {ResourceType: "recorders", Action: "control"},

		ServicePath("DirectoryIngestService", "StreamCameraState"):   {ResourceType: "directory", Action: "ingest"},
		ServicePath("DirectoryIngestService", "PublishSegmentIndex"): {ResourceType: "directory", Action: "ingest"},
		ServicePath("DirectoryIngestService", "PublishAIEvents"):     {ResourceType: "directory", Action: "ingest"},

		ServicePath("CrossTenantService", "ListAccessibleCustomers"): {ResourceType: "cross_tenant", Action: "read"},
		ServicePath("CrossTenantService", "MintDelegatedToken"):      {ResourceType: "cross_tenant", Action: "mint"},

		ServicePath("TenantProvisioningService", "CreateTenant"):  {ResourceType: "tenants", Action: "create"},
		ServicePath("TenantProvisioningService", "SuspendTenant"): {ResourceType: "tenants", Action: "suspend"},
	}
}
