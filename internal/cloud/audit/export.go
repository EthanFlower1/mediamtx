package audit

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
)

// exportEntries streams every page of Query through w in the requested
// format. It is shared by MemoryRecorder and SQLRecorder so both emit the
// exact same bytes for a given filter.
func exportEntries(ctx context.Context, r Recorder, filter QueryFilter, format ExportFormat, w io.Writer) error {
	if err := filter.Validate(); err != nil {
		return err
	}

	// Page size used when the caller has no explicit limit. 500 balances
	// memory against round-trips on the SQL backend.
	const pageSize = 500
	page := filter
	if page.Limit == 0 || page.Limit > pageSize {
		page.Limit = pageSize
	}
	remaining := filter.Limit // 0 = unlimited

	var (
		csvW     *csv.Writer
		jsonEnc  *json.Encoder
		headered bool
	)
	switch format {
	case ExportCSV:
		csvW = csv.NewWriter(w)
		defer csvW.Flush()
	case ExportJSON:
		jsonEnc = json.NewEncoder(w)
	default:
		return fmt.Errorf("audit: unknown export format %q", format)
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		entries, err := r.Query(ctx, page)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			break
		}
		for _, e := range entries {
			switch format {
			case ExportCSV:
				if !headered {
					if err := csvW.Write(csvHeader); err != nil {
						return err
					}
					headered = true
				}
				if err := csvW.Write(csvRow(e)); err != nil {
					return err
				}
			case ExportJSON:
				if err := jsonEnc.Encode(e); err != nil {
					return err
				}
			}
			if remaining > 0 {
				remaining--
				if remaining == 0 {
					if csvW != nil {
						csvW.Flush()
					}
					return nil
				}
			}
		}
		// Short page = we hit the end.
		if len(entries) < page.Limit {
			break
		}
		page.Cursor = entries[len(entries)-1].ID
	}
	if csvW != nil {
		csvW.Flush()
		return csvW.Error()
	}
	return nil
}

var csvHeader = []string{
	"id", "tenant_id", "actor_user_id", "actor_agent",
	"impersonating_user_id", "impersonated_tenant_id",
	"action", "resource_type", "resource_id",
	"result", "error_code", "ip_address", "user_agent", "request_id",
	"timestamp",
}

func csvRow(e Entry) []string {
	imperUser := ""
	if e.ImpersonatingUserID != nil {
		imperUser = *e.ImpersonatingUserID
	}
	imperTenant := ""
	if e.ImpersonatedTenantID != nil {
		imperTenant = *e.ImpersonatedTenantID
	}
	errCode := ""
	if e.ErrorCode != nil {
		errCode = *e.ErrorCode
	}
	return []string{
		e.ID, e.TenantID, e.ActorUserID, string(e.ActorAgent),
		imperUser, imperTenant,
		e.Action, e.ResourceType, e.ResourceID,
		string(e.Result), errCode, e.IPAddress, e.UserAgent, e.RequestID,
		e.Timestamp.UTC().Format("2006-01-02T15:04:05.000000Z07:00"),
	}
}
