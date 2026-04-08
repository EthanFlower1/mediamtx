// Package permissions is the MediaMTX cloud control plane's authorization
// engine (KAI-225). It wraps Casbin with a multi-tenant subject/object model
// and a fail-closed enforcer.
//
// See model.conf and README.md for the subject/object format.
package permissions

// Canonical action set. Every cloud API authorization check MUST use one of
// these constants — adding an action means adding a constant here.
//
// Wildcard matching: the Casbin keyMatch2 matcher treats "*" as "all actions"
// and "view.*" as "all view.* actions", so role templates can use the parent
// namespace instead of enumerating children.
const (
	// Live / recorded media viewing.
	ActionViewThumbnails = "view.thumbnails"
	ActionViewLive       = "view.live"
	ActionViewPlayback   = "view.playback"
	ActionViewSnapshot   = "view.snapshot"

	// Camera interaction.
	ActionPTZControl   = "ptz.control"
	ActionAudioTalkback = "audio.talkback"

	// Camera lifecycle.
	ActionCamerasAdd    = "cameras.add"
	ActionCamerasEdit   = "cameras.edit"
	ActionCamerasDelete = "cameras.delete"
	ActionCamerasMove   = "cameras.move"

	// User management.
	ActionUsersView        = "users.view"
	ActionUsersCreate      = "users.create"
	ActionUsersEdit        = "users.edit"
	ActionUsersDelete      = "users.delete"
	ActionUsersImpersonate = "users.impersonate"

	// Permission management.
	ActionPermissionsGrant  = "permissions.grant"
	ActionPermissionsRevoke = "permissions.revoke"

	// Platform configuration.
	ActionIntegrationsConfigure = "integrations.configure"
	ActionFederationConfigure   = "federation.configure"
	ActionBillingView           = "billing.view"
	ActionBillingChange         = "billing.change"

	// Observability / settings.
	ActionAuditRead    = "audit.read"
	ActionSystemHealth = "system.health"
	ActionSettingsEdit = "settings.edit"

	// Recorder lifecycle.
	ActionRecorderPair   = "recorder.pair"
	ActionRecorderUnpair = "recorder.unpair"

	// Tenant provisioning (KAI-227). These live on the "platform" meta-tenant
	// because creating an integrator or a customer tenant is, by definition,
	// a cross-tenant operation performed by platform staff or a parent
	// reseller granted `integrators.create_subreseller`.
	ActionIntegratorsCreate            = "integrators.create"
	ActionIntegratorsCreateSubReseller = "integrators.create_subreseller"
	ActionCustomerTenantsCreate        = "customer_tenants.create"
	ActionTenantsInviteAdmin           = "tenants.invite_admin"

	// AI / face vault.
	ActionAIConfigure      = "ai.configure"
	ActionAIFaceVaultRead  = "ai.facevault.read"
	ActionAIFaceVaultWrite = "ai.facevault.write"
	ActionAIFaceVaultErase = "ai.facevault.erase"
	ActionAIModelsUpload   = "ai.models.upload"

	// Behavioral analytics config (KAI-429).
	// behavioral.config.read  — viewer role + above may read detector configs.
	// behavioral.config.write — admin role may create, update, or delete configs.
	ActionBehavioralConfigRead  = "behavioral.config.read"
	ActionBehavioralConfigWrite = "behavioral.config.write"

	// Customer-integrator relationships (KAI-228, KAI-229).
	// relationships.read   — list or inspect relationships for a tenant.
	// relationships.write  — update permissions / markup on an existing relationship.
	// relationships.grant  — create or approve a new relationship, or revoke one.
	ActionRelationshipsRead  = "relationships.read"
	ActionRelationshipsWrite = "relationships.write"
	ActionRelationshipsGrant = "relationships.grant"
)

// AllActions is the canonical list of every action constant in this package.
// Role templates and tests use it; keep it in sync with the constants above.
var AllActions = []string{
	ActionViewThumbnails, ActionViewLive, ActionViewPlayback, ActionViewSnapshot,
	ActionPTZControl, ActionAudioTalkback,
	ActionCamerasAdd, ActionCamerasEdit, ActionCamerasDelete, ActionCamerasMove,
	ActionUsersView, ActionUsersCreate, ActionUsersEdit, ActionUsersDelete, ActionUsersImpersonate,
	ActionPermissionsGrant, ActionPermissionsRevoke,
	ActionIntegrationsConfigure, ActionFederationConfigure,
	ActionBillingView, ActionBillingChange,
	ActionAuditRead, ActionSystemHealth, ActionSettingsEdit,
	ActionRecorderPair, ActionRecorderUnpair,
	ActionIntegratorsCreate, ActionIntegratorsCreateSubReseller, ActionCustomerTenantsCreate, ActionTenantsInviteAdmin,
	ActionAIConfigure, ActionAIFaceVaultRead, ActionAIFaceVaultWrite, ActionAIFaceVaultErase, ActionAIModelsUpload,
	ActionRelationshipsRead, ActionRelationshipsWrite, ActionRelationshipsGrant,
	ActionBehavioralConfigRead, ActionBehavioralConfigWrite,
}
