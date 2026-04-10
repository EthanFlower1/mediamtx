package plans

// FeatureFlag is a stable string identifier for a capability that may be
// included in a plan tier or unlocked by an add-on. Feature flags are part of
// the package's public API: downstream code (authorization checks, UI gates,
// billing exports) compares against these constants, so values must remain
// stable across releases.
type FeatureFlag string

// Detection / analytics features.
const (
	// FeatureBasicDetection covers motion + simple object presence detection
	// included in every paid tier and the Free tier.
	FeatureBasicDetection FeatureFlag = "basic_detection"

	// FeatureFullObjectDetection is the production object detection model
	// (people, vehicles, animals, packages) included from Starter upward.
	FeatureFullObjectDetection FeatureFlag = "full_object_detection"

	// FeatureFaceRecognition unlocks the face recognition pipeline. Available
	// as part of Professional/Enterprise tiers and as a paid add-on.
	FeatureFaceRecognition FeatureFlag = "face_recognition"

	// FeatureLPR unlocks license plate recognition. Available as part of
	// Professional/Enterprise tiers and as a paid add-on.
	FeatureLPR FeatureFlag = "lpr"

	// FeatureBehavioralAnalytics covers loitering, line-cross, intrusion zone
	// and similar higher-order analytics.
	FeatureBehavioralAnalytics FeatureFlag = "behavioral_analytics"

	// FeatureCustomAIModelUpload allows tenants to upload their own ONNX
	// detection models. Enterprise-tier or via add-on.
	FeatureCustomAIModelUpload FeatureFlag = "custom_ai_model_upload"
)

// Identity / collaboration features.
const (
	// FeatureSSO enables SAML/OIDC single sign-on. Starter and above.
	FeatureSSO FeatureFlag = "sso"

	// FeatureUnlimitedUsers removes the per-tenant user cap. Starter and above.
	FeatureUnlimitedUsers FeatureFlag = "unlimited_users"
)

// Multi-tenant / partner features.
const (
	// FeatureFederation enables cross-tenant federation between sites a single
	// integrator manages. Professional and above (limited fanout).
	FeatureFederation FeatureFlag = "federation"

	// FeatureFederationUnlimited removes the federation fanout cap.
	// Enterprise tier or via add-on.
	FeatureFederationUnlimited FeatureFlag = "federation_unlimited"

	// FeatureIntegrations enables third-party integrations (webhooks, CRM,
	// access control, alarm panels). Professional and above.
	FeatureIntegrations FeatureFlag = "integrations"
)

// Storage / retention features.
const (
	// FeatureCloudArchiveExtended unlocks the ability to retain footage beyond
	// the plan's default retention window. Always paired with the
	// cloud_archive_extended add-on which carries per-GB-month pricing.
	FeatureCloudArchiveExtended FeatureFlag = "cloud_archive_extended"
)

// Operational / support features.
const (
	// FeaturePrioritySupport unlocks the SLA-backed support queue.
	// Enterprise tier or via add-on.
	FeaturePrioritySupport FeatureFlag = "priority_support"

	// FeatureDedicatedInferencePool reserves dedicated GPU capacity for the
	// tenant rather than the shared pool. Enterprise tier or via add-on.
	FeatureDedicatedInferencePool FeatureFlag = "dedicated_inference_pool"
)

// Compliance / deployment features.
const (
	// FeatureFedRAMP signals FedRAMP-eligible deployment posture. Enterprise
	// only; cannot be unlocked via add-on.
	FeatureFedRAMP FeatureFlag = "fedramp"

	// FeatureOnPremDeployment signals support for on-premise deployment.
	// Enterprise only.
	FeatureOnPremDeployment FeatureFlag = "on_prem_deployment"
)
