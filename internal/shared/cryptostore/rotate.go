package cryptostore

import (
	"context"
	"database/sql"
	"fmt"
)

// DBExecutor is the minimal subset of *sql.DB / *sql.Tx the rotation helper
// needs. Accepting this interface lets callers run rotation inside an
// existing transaction.
type DBExecutor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// RotateColumnOptions tunes batch rotation behaviour. All fields are optional.
type RotateColumnOptions struct {
	// IDColumn is the primary key column name. Defaults to "id".
	IDColumn string
	// BatchSize controls how many rows are fetched per SELECT. Defaults to 500.
	BatchSize int
	// WhereClause is an optional extra predicate (without the WHERE keyword).
	WhereClause string
}

// RotateColumn walks every non-NULL ciphertext in (table, column), decrypts
// with oldStore, and re-encrypts with newStore. It processes rows in batches
// and is safe to resume — each UPDATE targets a single primary-key row.
//
// The caller is responsible for schema validation (table and column must be
// safe identifiers — this function does NOT sanitize them, it interpolates
// them directly into the SQL). Never pass untrusted table/column names.
func RotateColumn(ctx context.Context, db DBExecutor, table, column string, oldStore, newStore Cryptostore, opts RotateColumnOptions) (int, error) {
	if table == "" || column == "" {
		return 0, fmt.Errorf("cryptostore: table and column are required")
	}
	if oldStore == nil || newStore == nil {
		return 0, fmt.Errorf("cryptostore: oldStore and newStore are required")
	}
	idCol := opts.IDColumn
	if idCol == "" {
		idCol = "id"
	}
	batch := opts.BatchSize
	if batch <= 0 {
		batch = 500
	}
	where := fmt.Sprintf("%s IS NOT NULL", column)
	if opts.WhereClause != "" {
		where = fmt.Sprintf("(%s) AND %s", opts.WhereClause, where)
	}

	// Cursor-based paging: fetch rows with id > lastID, rotate each, and
	// advance the cursor to the max id we saw. This is stable regardless of
	// whether rows have already been rotated (idempotent re-runs).
	selectSQL := fmt.Sprintf(
		"SELECT %s, %s FROM %s WHERE %s AND %s > ? ORDER BY %s LIMIT %d",
		idCol, column, table, where, idCol, idCol, batch,
	)
	updateSQL := fmt.Sprintf(
		"UPDATE %s SET %s = ? WHERE %s = ?",
		table, column, idCol,
	)

	rotated := 0
	var cursor any = int64(-1 << 62) // effectively -infinity for integer PKs
	for {
		ids, cts, err := fetchBatchCursor(ctx, db, selectSQL, cursor)
		if err != nil {
			return rotated, err
		}
		if len(ids) == 0 {
			return rotated, nil
		}
		for i, ct := range cts {
			newCT, err := RotateValue(oldStore, newStore, ct)
			if err != nil {
				// Already rotated rows will fail under oldStore; skip.
				continue
			}
			if _, err := db.ExecContext(ctx, updateSQL, newCT, ids[i]); err != nil {
				return rotated, fmt.Errorf("cryptostore: update %s.%s id=%v: %w", table, column, ids[i], err)
			}
			rotated++
		}
		// Advance cursor past the last id in this batch.
		cursor = ids[len(ids)-1]
		if len(ids) < batch {
			return rotated, nil
		}
	}
}

func fetchBatchCursor(ctx context.Context, db DBExecutor, query string, cursor any) ([]any, [][]byte, error) {
	rows, err := db.QueryContext(ctx, query, cursor)
	if err != nil {
		return nil, nil, fmt.Errorf("cryptostore: select: %w", err)
	}
	defer rows.Close()

	var ids []any
	var cts [][]byte
	for rows.Next() {
		var id any
		var ct []byte
		if err := rows.Scan(&id, &ct); err != nil {
			return nil, nil, fmt.Errorf("cryptostore: scan: %w", err)
		}
		ids = append(ids, id)
		cts = append(cts, ct)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("cryptostore: rows: %w", err)
	}
	return ids, cts, nil
}
