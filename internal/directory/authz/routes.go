package authz

// DirectoryRoutes is the canonical route table for all authenticated Directory
// API endpoints. Each entry maps an HTTP method + path prefix to the Casbin
// action and resource type required.
//
// Unauthenticated endpoints (POST /api/v1/pairing/request, GET .../token) are
// NOT in this table — they must be registered outside the authz middleware.
//
// This table is the single source of truth for "which permission does this
// endpoint require?" It is intentionally defined in data, not scattered across
// handler registration code, so that auditing and testing the permission matrix
// is straightforward.
var DirectoryRoutes = RouteTable{
	// --- Pairing (admin-only) ---
	{Method: "POST", PathPrefix: "/api/v1/pairing/tokens", Route: Route{
		Action: "recorder.pair", ResourceType: "recorders",
	}},
	{Method: "GET", PathPrefix: "/api/v1/pairing/pending", Route: Route{
		Action: "recorder.pair", ResourceType: "recorders",
	}},
	{Method: "POST", PathPrefix: "/api/v1/pairing/pending/", Route: Route{
		Action: "recorder.pair", ResourceType: "recorders",
	}},

	// --- Revocation (admin-only, KAI-158) ---
	{Method: "POST", PathPrefix: "/api/v1/admin/recorders/", Route: Route{
		Action: "recorder.unpair", ResourceType: "recorders",
		ResourceIDParam: "recorder_id",
	}},

	// --- Cameras ---
	{Method: "GET", PathPrefix: "/api/v1/cameras", Route: Route{
		Action: "view.thumbnails", ResourceType: "cameras",
	}},
	{Method: "POST", PathPrefix: "/api/v1/cameras", Route: Route{
		Action: "cameras.add", ResourceType: "cameras",
	}},
	{Method: "PUT", PathPrefix: "/api/v1/cameras/", Route: Route{
		Action: "cameras.edit", ResourceType: "cameras",
		ResourceIDParam: "camera_id",
	}},
	{Method: "DELETE", PathPrefix: "/api/v1/cameras/", Route: Route{
		Action: "cameras.delete", ResourceType: "cameras",
		ResourceIDParam: "camera_id",
	}},

	// --- Users ---
	{Method: "GET", PathPrefix: "/api/v1/users", Route: Route{
		Action: "users.view", ResourceType: "users",
	}},
	{Method: "POST", PathPrefix: "/api/v1/users", Route: Route{
		Action: "users.create", ResourceType: "users",
	}},
	{Method: "PUT", PathPrefix: "/api/v1/users/", Route: Route{
		Action: "users.edit", ResourceType: "users",
		ResourceIDParam: "user_id",
	}},
	{Method: "DELETE", PathPrefix: "/api/v1/users/", Route: Route{
		Action: "users.delete", ResourceType: "users",
		ResourceIDParam: "user_id",
	}},

	// --- Permissions ---
	{Method: "POST", PathPrefix: "/api/v1/permissions/grant", Route: Route{
		Action: "permissions.grant", ResourceType: "permissions",
	}},
	{Method: "POST", PathPrefix: "/api/v1/permissions/revoke", Route: Route{
		Action: "permissions.revoke", ResourceType: "permissions",
	}},

	// --- Settings ---
	{Method: "GET", PathPrefix: "/api/v1/settings", Route: Route{
		Action: "settings.edit", ResourceType: "settings",
	}},
	{Method: "PUT", PathPrefix: "/api/v1/settings", Route: Route{
		Action: "settings.edit", ResourceType: "settings",
	}},

	// --- System health ---
	{Method: "GET", PathPrefix: "/api/v1/health", Route: Route{
		Action: "system.health", ResourceType: "system",
	}},

	// --- Audit log ---
	{Method: "GET", PathPrefix: "/api/v1/audit", Route: Route{
		Action: "audit.read", ResourceType: "audit",
	}},

	// --- Recorders ---
	{Method: "GET", PathPrefix: "/api/v1/recorders", Route: Route{
		Action: "view.thumbnails", ResourceType: "recorders",
	}},

	// --- Streams ---
	{Method: "GET", PathPrefix: "/api/v1/streams", Route: Route{
		Action: "view.live", ResourceType: "streams",
	}},
	{Method: "POST", PathPrefix: "/api/v1/streams/request", Route: Route{
		Action: "view.live", ResourceType: "streams",
	}},

	// --- AI / Face vault ---
	{Method: "GET", PathPrefix: "/api/v1/ai/config", Route: Route{
		Action: "ai.configure", ResourceType: "ai",
	}},
	{Method: "PUT", PathPrefix: "/api/v1/ai/config", Route: Route{
		Action: "ai.configure", ResourceType: "ai",
	}},
	{Method: "GET", PathPrefix: "/api/v1/ai/facevault", Route: Route{
		Action: "ai.facevault.read", ResourceType: "ai",
	}},
	{Method: "POST", PathPrefix: "/api/v1/ai/facevault", Route: Route{
		Action: "ai.facevault.write", ResourceType: "ai",
	}},
	{Method: "DELETE", PathPrefix: "/api/v1/ai/facevault/", Route: Route{
		Action: "ai.facevault.erase", ResourceType: "ai",
	}},

	// --- Federation ---
	{Method: "GET", PathPrefix: "/api/v1/federation", Route: Route{
		Action: "federation.configure", ResourceType: "federation",
	}},
	{Method: "PUT", PathPrefix: "/api/v1/federation", Route: Route{
		Action: "federation.configure", ResourceType: "federation",
	}},

	// --- Federation streams (cross-site playback, KAI-274) ---
	{Method: "POST", PathPrefix: "/api/v1/federation/streams/request", Route: Route{
		Action: "view.live", ResourceType: "streams",
	}},
}
