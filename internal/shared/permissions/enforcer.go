package permissions

import (
	"context"
	_ "embed"
	"fmt"
	"sync"

	"github.com/casbin/casbin/v2"
	"github.com/casbin/casbin/v2/model"
)

//go:embed model.conf
var defaultModelConf string

// AuditSink receives one record per Enforce call. It is the seam for KAI-233's
// audit log; until that package lands, Enforcer uses a no-op default.
//
// TODO(KAI-233): replace this stub with the exported
// `internal/cloud/audit.Log` call once that package is merged.
type AuditSink interface {
	RecordEnforce(ctx context.Context, record AuditRecord)
}

// AuditRecord is a single authorization-decision log line.
type AuditRecord struct {
	Subject string
	Object  string
	Action  string
	Allowed bool
	Reason  string // optional human-readable note (e.g. "fail-closed: no policy")
}

// nopAuditSink discards records. Exported via DefaultAuditSink for callers
// that want to explicitly opt out.
type nopAuditSink struct{}

func (nopAuditSink) RecordEnforce(context.Context, AuditRecord) {}

// DefaultAuditSink is a no-op sink suitable for tests and until KAI-233 lands.
var DefaultAuditSink AuditSink = nopAuditSink{}

// Enforcer wraps a Casbin enforcer with the multi-tenant subject/object types
// from this package. Construct via NewEnforcer; do not embed casbin.Enforcer
// directly so callers can't bypass the Validate/fail-closed logic.
type Enforcer struct {
	mu    sync.RWMutex
	core  *casbin.Enforcer
	store PolicyStore
	audit AuditSink
}

// NewEnforcer builds an Enforcer backed by the given store. The store is
// loaded immediately; call ReloadPolicy after out-of-band store mutations.
func NewEnforcer(store PolicyStore, audit AuditSink) (*Enforcer, error) {
	if store == nil {
		return nil, fmt.Errorf("permissions: store is required")
	}
	if audit == nil {
		audit = DefaultAuditSink
	}

	m, err := model.NewModelFromString(defaultModelConf)
	if err != nil {
		return nil, fmt.Errorf("permissions: parse model: %w", err)
	}

	core, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, fmt.Errorf("permissions: new enforcer: %w", err)
	}
	// Fail-closed: Casbin's default effect policy already denies on miss
	// because no allow policy will match. We assert autoSave off since
	// persistence flows through the PolicyStore, not an attached adapter.
	core.EnableAutoSave(false)

	e := &Enforcer{core: core, store: store, audit: audit}
	if err := e.ReloadPolicy(); err != nil {
		return nil, err
	}
	return e, nil
}

// ReloadPolicy rebuilds the in-memory Casbin state from the store. Call this
// after any batch of out-of-band store mutations (the Add*/Remove* methods on
// Enforcer already reload implicitly).
func (e *Enforcer) ReloadPolicy() error {
	policies, groupings, err := e.store.LoadAll()
	if err != nil {
		return fmt.Errorf("permissions: load policies: %w", err)
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	e.core.ClearPolicy()
	for _, p := range policies {
		if _, err := e.core.AddPolicy(p.Sub, p.Obj, p.Act, p.effect()); err != nil {
			return fmt.Errorf("permissions: add policy: %w", err)
		}
	}
	for _, g := range groupings {
		if _, err := e.core.AddGroupingPolicy(g.Subject, g.Role); err != nil {
			return fmt.Errorf("permissions: add grouping: %w", err)
		}
	}
	// Rebuild the role manager's link cache. Required after ClearPolicy
	// because the RBAC resolver caches g edges separately from the model.
	if err := e.core.BuildRoleLinks(); err != nil {
		return fmt.Errorf("permissions: build role links: %w", err)
	}
	return nil
}

// Enforce is the single authorization entry point. It is fail-closed: any
// error or policy miss returns (false, err-or-nil).
func (e *Enforcer) Enforce(ctx context.Context, subject SubjectRef, object ObjectRef, action string) (bool, error) {
	if err := subject.Validate(); err != nil {
		e.audit.RecordEnforce(ctx, AuditRecord{
			Subject: subject.String(), Object: object.String(), Action: action,
			Allowed: false, Reason: "invalid subject: " + err.Error(),
		})
		return false, err
	}
	if err := object.Validate(); err != nil {
		e.audit.RecordEnforce(ctx, AuditRecord{
			Subject: subject.String(), Object: object.String(), Action: action,
			Allowed: false, Reason: "invalid object: " + err.Error(),
		})
		return false, err
	}
	if action == "" {
		e.audit.RecordEnforce(ctx, AuditRecord{
			Subject: subject.String(), Object: object.String(), Action: action,
			Allowed: false, Reason: "empty action",
		})
		return false, fmt.Errorf("permissions: action is empty")
	}

	e.mu.RLock()
	allowed, err := e.core.Enforce(subject.String(), object.String(), action)
	e.mu.RUnlock()

	rec := AuditRecord{
		Subject: subject.String(),
		Object:  object.String(),
		Action:  action,
		Allowed: allowed && err == nil,
	}
	if err != nil {
		rec.Reason = "enforcer error: " + err.Error()
	} else if !allowed {
		rec.Reason = "fail-closed: no matching allow policy"
	}
	e.audit.RecordEnforce(ctx, rec)

	if err != nil {
		return false, fmt.Errorf("permissions: enforce: %w", err)
	}
	return allowed, nil
}

// AddPolicy persists and activates a single policy rule.
func (e *Enforcer) AddPolicy(rule PolicyRule) error {
	if err := e.store.AddPolicy(rule); err != nil {
		return err
	}
	return e.ReloadPolicy()
}

// RemovePolicy persists and activates a single policy removal.
func (e *Enforcer) RemovePolicy(rule PolicyRule) error {
	if err := e.store.RemovePolicy(rule); err != nil {
		return err
	}
	return e.ReloadPolicy()
}

// AddGrouping persists and activates a single grouping rule.
func (e *Enforcer) AddGrouping(rule GroupingRule) error {
	if err := e.store.AddGrouping(rule); err != nil {
		return err
	}
	return e.ReloadPolicy()
}

// RemoveGrouping persists and activates a single grouping removal.
func (e *Enforcer) RemoveGrouping(rule GroupingRule) error {
	if err := e.store.RemoveGrouping(rule); err != nil {
		return err
	}
	return e.ReloadPolicy()
}

// Store exposes the underlying PolicyStore (used by tests and seed helpers).
func (e *Enforcer) Store() PolicyStore { return e.store }
