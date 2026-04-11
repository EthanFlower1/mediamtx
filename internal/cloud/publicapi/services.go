package publicapi

import (
	"errors"
	"net/http"
)

// PublicServicePath returns the canonical Connect path for a public API
// (service, method) pair. All public services live under the
// kaivue.v1.public package namespace.
func PublicServicePath(service, method string) string {
	return "/kaivue.v1.public." + service + "/" + method
}

// PublicRESTPath returns the versioned REST path for a resource endpoint.
func PublicRESTPath(resource string) string {
	return "/api/v1/" + resource
}

// PublicRESTPathWithID returns the versioned REST path for a specific resource.
func PublicRESTPathWithID(resource, id string) string {
	return "/api/v1/" + resource + "/" + id
}

// publicService describes one Connect-Go service and its methods for
// route registration.
type publicService struct {
	service string
	methods []string
}

// PublicServices is the canonical list of public API services and their
// methods. This is the contract that downstream tickets implement against.
// Keep in alphabetical order by service.
var PublicServices = []publicService{
	{"PublicCamerasService", []string{
		"CreateCamera",
		"GetCamera",
		"UpdateCamera",
		"DeleteCamera",
		"ListCameras",
	}},
	{"PublicEventsService", []string{
		"GetEvent",
		"ListEvents",
		"AcknowledgeEvent",
		"DeleteEvent",
	}},
	{"PublicIntegrationsService", []string{
		"CreateIntegration",
		"GetIntegration",
		"UpdateIntegration",
		"DeleteIntegration",
		"ListIntegrations",
		"TestIntegration",
	}},
	{"PublicRecordingsService", []string{
		"GetRecording",
		"ListRecordings",
		"DeleteRecording",
	}},
	{"PublicRetentionService", []string{
		"CreateRetentionPolicy",
		"GetRetentionPolicy",
		"UpdateRetentionPolicy",
		"DeleteRetentionPolicy",
		"ListRetentionPolicies",
	}},
	{"PublicSchedulesService", []string{
		"CreateSchedule",
		"GetSchedule",
		"UpdateSchedule",
		"DeleteSchedule",
		"ListSchedules",
	}},
	{"PublicUsersService", []string{
		"CreateUser",
		"GetUser",
		"UpdateUser",
		"DeleteUser",
		"ListUsers",
	}},
}

// PublicRouteAuthorization maps each public service method to the Casbin
// (resource, action) pair used by the permission middleware. This is the
// contract that downstream tickets use for ACL enforcement.
func PublicRouteAuthorizations() map[string]RouteAuth {
	return map[string]RouteAuth{
		// Cameras
		PublicServicePath("PublicCamerasService", "CreateCamera"):  {Resource: "cameras", Action: "create"},
		PublicServicePath("PublicCamerasService", "GetCamera"):     {Resource: "cameras", Action: "read"},
		PublicServicePath("PublicCamerasService", "UpdateCamera"):  {Resource: "cameras", Action: "update"},
		PublicServicePath("PublicCamerasService", "DeleteCamera"):  {Resource: "cameras", Action: "delete"},
		PublicServicePath("PublicCamerasService", "ListCameras"):   {Resource: "cameras", Action: "read"},

		// Users
		PublicServicePath("PublicUsersService", "CreateUser"):  {Resource: "users", Action: "create"},
		PublicServicePath("PublicUsersService", "GetUser"):     {Resource: "users", Action: "read"},
		PublicServicePath("PublicUsersService", "UpdateUser"):  {Resource: "users", Action: "update"},
		PublicServicePath("PublicUsersService", "DeleteUser"):  {Resource: "users", Action: "delete"},
		PublicServicePath("PublicUsersService", "ListUsers"):   {Resource: "users", Action: "read"},

		// Recordings
		PublicServicePath("PublicRecordingsService", "GetRecording"):    {Resource: "recordings", Action: "read"},
		PublicServicePath("PublicRecordingsService", "ListRecordings"):  {Resource: "recordings", Action: "read"},
		PublicServicePath("PublicRecordingsService", "DeleteRecording"): {Resource: "recordings", Action: "delete"},

		// Events
		PublicServicePath("PublicEventsService", "GetEvent"):          {Resource: "events", Action: "read"},
		PublicServicePath("PublicEventsService", "ListEvents"):        {Resource: "events", Action: "read"},
		PublicServicePath("PublicEventsService", "AcknowledgeEvent"):  {Resource: "events", Action: "update"},
		PublicServicePath("PublicEventsService", "DeleteEvent"):       {Resource: "events", Action: "delete"},

		// Schedules
		PublicServicePath("PublicSchedulesService", "CreateSchedule"):  {Resource: "schedules", Action: "create"},
		PublicServicePath("PublicSchedulesService", "GetSchedule"):     {Resource: "schedules", Action: "read"},
		PublicServicePath("PublicSchedulesService", "UpdateSchedule"):  {Resource: "schedules", Action: "update"},
		PublicServicePath("PublicSchedulesService", "DeleteSchedule"):  {Resource: "schedules", Action: "delete"},
		PublicServicePath("PublicSchedulesService", "ListSchedules"):   {Resource: "schedules", Action: "read"},

		// Retention
		PublicServicePath("PublicRetentionService", "CreateRetentionPolicy"):  {Resource: "retention", Action: "create"},
		PublicServicePath("PublicRetentionService", "GetRetentionPolicy"):     {Resource: "retention", Action: "read"},
		PublicServicePath("PublicRetentionService", "UpdateRetentionPolicy"):  {Resource: "retention", Action: "update"},
		PublicServicePath("PublicRetentionService", "DeleteRetentionPolicy"):  {Resource: "retention", Action: "delete"},
		PublicServicePath("PublicRetentionService", "ListRetentionPolicies"):  {Resource: "retention", Action: "read"},

		// Integrations
		PublicServicePath("PublicIntegrationsService", "CreateIntegration"):  {Resource: "integrations", Action: "create"},
		PublicServicePath("PublicIntegrationsService", "GetIntegration"):     {Resource: "integrations", Action: "read"},
		PublicServicePath("PublicIntegrationsService", "UpdateIntegration"):  {Resource: "integrations", Action: "update"},
		PublicServicePath("PublicIntegrationsService", "DeleteIntegration"):  {Resource: "integrations", Action: "delete"},
		PublicServicePath("PublicIntegrationsService", "ListIntegrations"):   {Resource: "integrations", Action: "read"},
		PublicServicePath("PublicIntegrationsService", "TestIntegration"):    {Resource: "integrations", Action: "test"},
	}
}

// RouteAuth describes the Casbin resource + action for a route.
type RouteAuth struct {
	Resource string
	Action   string
}

// unimplementedPublicHandler returns a Connect-style Unimplemented error.
// Downstream tickets replace these stubs with real implementations.
func unimplementedPublicHandler(service, method string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writePublicError(w, http.StatusNotImplemented, "unimplemented",
			"TODO(KAI-399): "+service+"."+method+" not yet implemented")
	})
}

// writePublicError writes a JSON error envelope matching the Connect wire
// format. The structure is identical to the apiserver error envelope so
// clients get a consistent experience across internal and public APIs.
func writePublicError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// Manual JSON to avoid import cycles with encoding/json.
	_, _ = w.Write([]byte(`{"code":"` + code + `","message":"` + escapeJSON(message) + `"}`))
}

// escapeJSON escapes double quotes and backslashes in a string for safe
// embedding in a JSON string literal.
func escapeJSON(s string) string {
	var out []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			out = append(out, '\\', '"')
		case '\\':
			out = append(out, '\\', '\\')
		case '\n':
			out = append(out, '\\', 'n')
		case '\r':
			out = append(out, '\\', 'r')
		case '\t':
			out = append(out, '\\', 't')
		default:
			out = append(out, s[i])
		}
	}
	return string(out)
}

// AllPublicPaths returns every registered public API path. Used by
// isolation tests to verify coverage.
func AllPublicPaths() []string {
	var paths []string
	for _, svc := range PublicServices {
		for _, m := range svc.methods {
			paths = append(paths, PublicServicePath(svc.service, m))
		}
	}
	return paths
}

// TotalPublicMethodCount returns the total number of public API methods.
func TotalPublicMethodCount() int {
	n := 0
	for _, svc := range PublicServices {
		n += len(svc.methods)
	}
	return n
}

// ErrUnimplemented is returned by stub handlers.
var ErrUnimplemented = errors.New("not implemented")
