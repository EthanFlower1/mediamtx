package db

import (
	"context"
	"fmt"
	"regexp"
)

// tenantIDSafe validates that a tenant_id is safe for use in dynamic DDL
// (partition names, index names). Only alphanumeric + hyphens + underscores
// are allowed. This prevents SQL injection in the DDL statements below where
// tenant_id is interpolated into identifiers (not parameterizable in Postgres).
var tenantIDSafe = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// ProvisionVectorPartitions creates per-tenant LIST partitions and HNSW
// indexes for face_embeddings, clip_embeddings, and consent_records. This
// MUST be called once per tenant at provisioning time (Postgres only).
//
// Seam #4: each tenant's embeddings live in a physically separate partition
// with a dedicated HNSW index. A query scoped to tenant_id hits only that
// partition's index — vectors from other tenants are unreachable even under
// a buggy WHERE clause.
//
// This is a no-op on SQLite (unit test mode) because the SQLite stubs are
// unpartitioned.
func (d *DB) ProvisionVectorPartitions(ctx context.Context, tenantID string) error {
	if d.dialect != DialectPostgres {
		return nil // no-op for SQLite test mode
	}
	if !tenantIDSafe.MatchString(tenantID) {
		return fmt.Errorf("invalid tenant_id for partition name: %q", tenantID)
	}

	// consent_records partition
	if err := d.createListPartition(ctx, "consent_records", tenantID); err != nil {
		return fmt.Errorf("consent_records partition: %w", err)
	}

	// face_embeddings partition + HNSW index
	if err := d.createListPartition(ctx, "face_embeddings", tenantID); err != nil {
		return fmt.Errorf("face_embeddings partition: %w", err)
	}
	if err := d.createHNSWIndex(ctx, "face_embeddings", tenantID, "embedding", "vector_cosine_ops", 16, 64); err != nil {
		return fmt.Errorf("face_embeddings HNSW index: %w", err)
	}

	// clip_embeddings partition + HNSW index
	if err := d.createListPartition(ctx, "clip_embeddings", tenantID); err != nil {
		return fmt.Errorf("clip_embeddings partition: %w", err)
	}
	if err := d.createHNSWIndex(ctx, "clip_embeddings", tenantID, "embedding", "vector_cosine_ops", 32, 64); err != nil {
		return fmt.Errorf("clip_embeddings HNSW index: %w", err)
	}

	return nil
}

// DropVectorPartitions removes per-tenant partitions. Used for tenant
// deprovisioning and rollback compensation. Cascades to indexes.
func (d *DB) DropVectorPartitions(ctx context.Context, tenantID string) error {
	if d.dialect != DialectPostgres {
		return nil
	}
	if !tenantIDSafe.MatchString(tenantID) {
		return fmt.Errorf("invalid tenant_id for partition name: %q", tenantID)
	}
	tables := []string{"clip_embeddings", "face_embeddings", "consent_records"}
	for _, table := range tables {
		partName := fmt.Sprintf("%s_t_%s", table, tenantID)
		ddl := fmt.Sprintf("DROP TABLE IF EXISTS %s", partName)
		if _, err := d.ExecContext(ctx, ddl); err != nil {
			return fmt.Errorf("drop partition %s: %w", partName, err)
		}
	}
	return nil
}

func (d *DB) createListPartition(ctx context.Context, parentTable, tenantID string) error {
	partName := fmt.Sprintf("%s_t_%s", parentTable, tenantID)
	ddl := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF %s FOR VALUES IN ('%s')",
		partName, parentTable, tenantID,
	)
	_, err := d.ExecContext(ctx, ddl)
	return err
}

func (d *DB) createHNSWIndex(ctx context.Context, table, tenantID, column, opsClass string, m, efConstruction int) error {
	partName := fmt.Sprintf("%s_t_%s", table, tenantID)
	idxName := fmt.Sprintf("idx_%s_hnsw", partName)
	ddl := fmt.Sprintf(
		"CREATE INDEX IF NOT EXISTS %s ON %s USING hnsw (%s %s) WITH (m = %d, ef_construction = %d)",
		idxName, partName, column, opsClass, m, efConstruction,
	)
	_, err := d.ExecContext(ctx, ddl)
	return err
}
