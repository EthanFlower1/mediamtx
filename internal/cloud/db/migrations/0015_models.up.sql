-- KAI-279: AI model registry for the cloud control plane.
-- Stores model metadata (not the model bytes themselves — those live in S3/R2).

CREATE TABLE IF NOT EXISTS models (
    id              TEXT PRIMARY KEY,
    tenant_id       TEXT NOT NULL,
    name            TEXT NOT NULL,
    version         TEXT NOT NULL,
    framework       TEXT NOT NULL CHECK (framework IN ('onnx', 'tensorrt', 'coreml', 'pytorch', 'tflite')),
    file_ref        TEXT NOT NULL,
    file_sha256     TEXT NOT NULL,
    size_bytes      BIGINT NOT NULL DEFAULT 0,
    metrics         JSONB NOT NULL DEFAULT '{}'::jsonb,
    approval_state  TEXT NOT NULL DEFAULT 'draft' CHECK (approval_state IN ('draft', 'in_review', 'approved', 'rejected', 'deprecated')),
    approved_by     TEXT,
    approved_at     TIMESTAMPTZ,
    owner_user_id   TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Enforce one version per (tenant, name, version) tuple.
CREATE UNIQUE INDEX IF NOT EXISTS idx_models_tenant_name_version
    ON models (tenant_id, name, version);

-- Fast lookup for ResolveApproved: tenant + name + state.
CREATE INDEX IF NOT EXISTS idx_models_resolve
    ON models (tenant_id, name, approval_state, created_at DESC);
