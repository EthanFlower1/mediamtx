package federation

import (
	"context"
	"errors"

	connect "connectrpc.com/connect"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/shared/permissions"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
)

// contextKey is an unexported type for context keys in this package.
type contextKey string

// PeerDirectoryIDKey is the context key that federation mTLS middleware uses
// to convey the authenticated peer's directory ID. Handlers extract it with
// PeerDirectoryIDFromContext.
const PeerDirectoryIDKey contextKey = "federation_peer_directory_id"

// PeerDirectoryIDFromContext extracts the peer directory ID that was set by
// the mTLS authentication middleware. Returns empty string if not present.
func PeerDirectoryIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(PeerDirectoryIDKey).(string)
	return v
}

// defaultPageSize is used when the client doesn't specify page_size.
const defaultPageSize = 50

// maxPageSize caps the maximum page size to prevent abuse.
const maxPageSize = 500

// clampPageSize normalises a client-supplied page_size to [1, maxPageSize].
func clampPageSize(requested int32) int32 {
	if requested <= 0 {
		return defaultPageSize
	}
	if requested > maxPageSize {
		return maxPageSize
	}
	return requested
}

// UserRecord is the local user representation that UserStore returns.
// It intentionally avoids importing proto types so the store layer stays
// transport-agnostic.
type UserRecord struct {
	ID          string
	TenantID    string
	Username    string
	Email       string
	DisplayName string
	Groups      []string
	Disabled    bool
}

// GroupRecord is the local group representation.
type GroupRecord struct {
	ID          string
	TenantID    string
	Name        string
	Description string
}

// CameraRecord is the local camera representation for federation catalog
// purposes. Credential references are intentionally excluded — the federation
// layer MUST NOT expose secrets to peers.
type CameraRecord struct {
	ID          string
	TenantID    string
	RecorderID  string
	Name        string
	Description string
	Location    string
	State       kaivuev1.CameraState
	Labels      []string
}

// UserStore provides access to the local user directory. Implementations are
// expected to return all users for the given tenant; Casbin filtering happens
// in the handler layer.
type UserStore interface {
	// ListUsers returns users for the tenant, optionally filtered by a search
	// string (substring match on username, email, or display name).
	ListUsers(ctx context.Context, tenantID string, search string) ([]UserRecord, error)
}

// GroupStore provides access to the local group directory.
type GroupStore interface {
	// ListGroups returns groups for the tenant.
	ListGroups(ctx context.Context, tenantID string) ([]GroupRecord, error)
}

// CameraStore provides access to the local camera registry.
type CameraStore interface {
	// ListCameras returns cameras for the tenant, optionally filtered by
	// recorder ID and/or a search string (substring match on name).
	ListCameras(ctx context.Context, tenantID string, recorderID string, search string) ([]CameraRecord, error)
}

// ListUsers returns users visible to the requesting federation peer, filtered
// by the Casbin grants for the peer's directory ID.
func (h *Handler) ListUsers(
	ctx context.Context,
	req *connect.Request[kaivuev1.ListUsersRequest],
) (*connect.Response[kaivuev1.ListUsersResponse], error) {
	peerID := PeerDirectoryIDFromContext(ctx)
	if peerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing peer directory identity"))
	}

	tenant := req.Msg.GetTenant()
	if tenant == nil || tenant.GetId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tenant is required"))
	}

	if h.cfg.UserStore == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("user store not configured"))
	}

	// Fetch all local users for the tenant.
	users, err := h.cfg.UserStore.ListUsers(ctx, tenant.GetId(), req.Msg.GetSearch())
	if err != nil {
		h.log.ErrorContext(ctx, "ListUsers: store error", "error", err, "peer", peerID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list users"))
	}

	// Filter by Casbin grants: the peer must have a grant on "users/<user_id>"
	// with at least ActionUsersView.
	authTenant := auth.TenantRef{ID: tenant.GetId()}
	sub := permissions.NewFederationSubject(peerID)
	var filtered []UserRecord
	for _, u := range users {
		obj := permissions.NewObject(authTenant, "users", u.ID)
		allowed, enforceErr := h.cfg.GrantManager.Enforcer().Enforce(ctx, sub, obj, permissions.ActionUsersView)
		if enforceErr != nil {
			h.log.WarnContext(ctx, "ListUsers: enforce error", "error", enforceErr, "user_id", u.ID)
			continue
		}
		if allowed {
			filtered = append(filtered, u)
		}
	}

	// Pagination.
	pageSize := clampPageSize(req.Msg.GetPageSize())
	start := 0
	if cursor := req.Msg.GetCursor(); cursor != "" {
		start = findCursorIndex(filtered, cursor, func(r UserRecord) string { return r.ID })
	}

	end := start + int(pageSize)
	if end > len(filtered) {
		end = len(filtered)
	}

	page := filtered[start:end]
	var nextCursor string
	if end < len(filtered) {
		nextCursor = page[len(page)-1].ID
	}

	// Convert to proto.
	protoUsers := make([]*kaivuev1.FederatedUser, 0, len(page))
	for _, u := range page {
		protoUsers = append(protoUsers, &kaivuev1.FederatedUser{
			Id:          u.ID,
			Tenant:      tenant,
			Username:    u.Username,
			Email:       u.Email,
			DisplayName: u.DisplayName,
			Groups:      u.Groups,
			Disabled:    u.Disabled,
		})
	}

	h.log.DebugContext(ctx, "ListUsers",
		"peer", peerID,
		"tenant", tenant.GetId(),
		"total_filtered", len(filtered),
		"page_size", len(page),
	)

	return connect.NewResponse(&kaivuev1.ListUsersResponse{
		Users:      protoUsers,
		NextCursor: nextCursor,
	}), nil
}

// ListGroups returns groups visible to the requesting federation peer.
func (h *Handler) ListGroups(
	ctx context.Context,
	req *connect.Request[kaivuev1.ListGroupsRequest],
) (*connect.Response[kaivuev1.ListGroupsResponse], error) {
	peerID := PeerDirectoryIDFromContext(ctx)
	if peerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing peer directory identity"))
	}

	tenant := req.Msg.GetTenant()
	if tenant == nil || tenant.GetId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tenant is required"))
	}

	if h.cfg.GroupStore == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("group store not configured"))
	}

	groups, err := h.cfg.GroupStore.ListGroups(ctx, tenant.GetId())
	if err != nil {
		h.log.ErrorContext(ctx, "ListGroups: store error", "error", err, "peer", peerID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list groups"))
	}

	// Filter by Casbin grants: peer must have ActionUsersView on "groups/<id>".
	authTenant := auth.TenantRef{ID: tenant.GetId()}
	sub := permissions.NewFederationSubject(peerID)
	var filtered []GroupRecord
	for _, g := range groups {
		obj := permissions.NewObject(authTenant, "groups", g.ID)
		allowed, enforceErr := h.cfg.GrantManager.Enforcer().Enforce(ctx, sub, obj, permissions.ActionUsersView)
		if enforceErr != nil {
			h.log.WarnContext(ctx, "ListGroups: enforce error", "error", enforceErr, "group_id", g.ID)
			continue
		}
		if allowed {
			filtered = append(filtered, g)
		}
	}

	// Pagination.
	pageSize := clampPageSize(req.Msg.GetPageSize())
	start := 0
	if cursor := req.Msg.GetCursor(); cursor != "" {
		start = findCursorIndex(filtered, cursor, func(r GroupRecord) string { return r.ID })
	}

	end := start + int(pageSize)
	if end > len(filtered) {
		end = len(filtered)
	}

	page := filtered[start:end]
	var nextCursor string
	if end < len(filtered) {
		nextCursor = page[len(page)-1].ID
	}

	protoGroups := make([]*kaivuev1.FederatedGroup, 0, len(page))
	for _, g := range page {
		protoGroups = append(protoGroups, &kaivuev1.FederatedGroup{
			Id:          g.ID,
			Tenant:      tenant,
			Name:        g.Name,
			Description: g.Description,
		})
	}

	h.log.DebugContext(ctx, "ListGroups",
		"peer", peerID,
		"tenant", tenant.GetId(),
		"total_filtered", len(filtered),
		"page_size", len(page),
	)

	return connect.NewResponse(&kaivuev1.ListGroupsResponse{
		Groups:     protoGroups,
		NextCursor: nextCursor,
	}), nil
}

// ListCameras returns cameras visible to the requesting federation peer,
// with metadata (name, location, status). Credential references are scrubbed.
func (h *Handler) ListCameras(
	ctx context.Context,
	req *connect.Request[kaivuev1.FederationPeerServiceListCamerasRequest],
) (*connect.Response[kaivuev1.FederationPeerServiceListCamerasResponse], error) {
	peerID := PeerDirectoryIDFromContext(ctx)
	if peerID == "" {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("missing peer directory identity"))
	}

	tenant := req.Msg.GetTenant()
	if tenant == nil || tenant.GetId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("tenant is required"))
	}

	if h.cfg.CameraStore == nil {
		return nil, connect.NewError(connect.CodeUnimplemented, errors.New("camera store not configured"))
	}

	cameras, err := h.cfg.CameraStore.ListCameras(
		ctx,
		tenant.GetId(),
		req.Msg.GetRecorderId(),
		req.Msg.GetSearch(),
	)
	if err != nil {
		h.log.ErrorContext(ctx, "ListCameras: store error", "error", err, "peer", peerID)
		return nil, connect.NewError(connect.CodeInternal, errors.New("failed to list cameras"))
	}

	// Filter by Casbin grants: peer must have ActionViewLive (or any view action)
	// on "cameras/<camera_id>".
	authTenant := auth.TenantRef{ID: tenant.GetId()}
	sub := permissions.NewFederationSubject(peerID)
	var filtered []CameraRecord
	for _, c := range cameras {
		obj := permissions.NewObject(authTenant, "cameras", c.ID)
		// Check if the peer has ANY allowed federation action on this camera.
		if hasAnyGrant(ctx, h.cfg.GrantManager.Enforcer(), sub, obj) {
			filtered = append(filtered, c)
		}
	}

	// Pagination.
	pageSize := clampPageSize(req.Msg.GetPageSize())
	start := 0
	if cursor := req.Msg.GetCursor(); cursor != "" {
		start = findCursorIndex(filtered, cursor, func(r CameraRecord) string { return r.ID })
	}

	end := start + int(pageSize)
	if end > len(filtered) {
		end = len(filtered)
	}

	page := filtered[start:end]
	var nextCursor string
	if end < len(filtered) {
		nextCursor = page[len(page)-1].ID
	}

	// Convert to proto Camera, scrubbing credential_ref.
	protoCameras := make([]*kaivuev1.Camera, 0, len(page))
	for _, c := range page {
		protoCameras = append(protoCameras, &kaivuev1.Camera{
			Id:          c.ID,
			Tenant:      tenant,
			RecorderId:  c.RecorderID,
			Name:        c.Name,
			Description: c.Description,
			State:       c.State,
			Labels:      c.Labels,
			// credential_ref intentionally omitted — federation peers MUST NOT
			// receive credential material.
		})
	}

	h.log.DebugContext(ctx, "ListCameras",
		"peer", peerID,
		"tenant", tenant.GetId(),
		"total_filtered", len(filtered),
		"page_size", len(page),
	)

	return connect.NewResponse(&kaivuev1.FederationPeerServiceListCamerasResponse{
		Cameras:    protoCameras,
		NextCursor: nextCursor,
	}), nil
}

// hasAnyGrant checks if the subject has any federation-allowed action on the
// given object. This is used for camera visibility — if the peer has ANY grant
// (view.live, view.playback, etc.) they can see the camera in the catalog.
func hasAnyGrant(ctx context.Context, enforcer *permissions.Enforcer, sub permissions.SubjectRef, obj permissions.ObjectRef) bool {
	for action := range permissions.FederationAllowedActions {
		allowed, err := enforcer.Enforce(ctx, sub, obj, action)
		if err == nil && allowed {
			return true
		}
	}
	return false
}

// findCursorIndex returns the index of the first element AFTER the cursor
// match in a slice. The idFunc extracts the ID from each element. If the
// cursor is not found, pagination starts from 0 (graceful degradation).
func findCursorIndex[T any](items []T, cursor string, idFunc func(T) string) int {
	for i, item := range items {
		if idFunc(item) == cursor {
			return i + 1
		}
	}
	return 0
}

