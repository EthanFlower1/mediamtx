package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// UserHandler implements HTTP endpoints for user management.
type UserHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// userCreateRequest is the JSON body for creating a user.
type userCreateRequest struct {
	Username          string `json:"username" binding:"required"`
	Password          string `json:"password" binding:"required"`
	Role              string `json:"role"`
	RoleID            string `json:"role_id"`
	CameraPermissions string `json:"camera_permissions"`
}

// userUpdateRequest is the JSON body for updating a user.
type userUpdateRequest struct {
	Username          string `json:"username"`
	Password          string `json:"password"`
	Role              string `json:"role"`
	RoleID            string `json:"role_id"`
	CameraPermissions string `json:"camera_permissions"`
}

// requireAdmin checks that the requesting user has the admin role.
// Returns true if the request should proceed, false if it was aborted.
func requireAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	if role != "admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return false
	}
	return true
}

// List returns all users as a JSON array.
func (h *UserHandler) List(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	users, err := h.DB.ListUsers()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list users", err)
		return
	}
	if users == nil {
		users = []*db.User{}
	}
	c.JSON(http.StatusOK, users)
}

// Get returns a single user by ID.
func (h *UserHandler) Get(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")
	user, err := h.DB.GetUser(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve user", err)
		return
	}
	c.JSON(http.StatusOK, user)
}

// Create creates a new user with a hashed password.
func (h *UserHandler) Create(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req userCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	hashed, err := hashPassword(req.Password)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to hash password", err)
		return
	}

	user := &db.User{
		Username:          req.Username,
		PasswordHash:      hashed,
		Role:              req.Role,
		RoleID:            req.RoleID,
		CameraPermissions: req.CameraPermissions,
	}

	if user.Role == "" {
		user.Role = "viewer"
	}

	if err := h.DB.CreateUser(user); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create user", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "create", "user", user.ID, "Created user "+user.Username+" with role "+user.Role)
	}

	c.JSON(http.StatusCreated, user)
}

// Update updates an existing user. If the password is changed, all refresh
// tokens for the user are revoked.
func (h *UserHandler) Update(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")

	existing, err := h.DB.GetUser(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve user for update", err)
		return
	}

	var req userUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	passwordChanged := false

	if req.Username != "" {
		existing.Username = req.Username
	}
	if req.Password != "" {
		hashed, err := hashPassword(req.Password)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "failed to hash password", err)
			return
		}
		existing.PasswordHash = hashed
		passwordChanged = true
	}
	if req.Role != "" {
		existing.Role = req.Role
	}
	if req.RoleID != "" {
		existing.RoleID = req.RoleID
	}
	if req.CameraPermissions != "" {
		existing.CameraPermissions = req.CameraPermissions
	}

	if err := h.DB.UpdateUser(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update user", err)
		return
	}

	// Revoke all tokens if password was changed.
	if passwordChanged {
		_ = h.DB.RevokeAllUserTokens(existing.ID)
	}

	if h.Audit != nil {
		details := "Updated user " + existing.Username
		if passwordChanged {
			details += " (password changed)"
		}
		h.Audit.logAction(c, "update", "user", existing.ID, details)
	}

	c.JSON(http.StatusOK, existing)
}

// passwordChangeRequest is the JSON body for changing a user's own password.
type passwordChangeRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required"`
}

// ChangePassword lets an authenticated user change their own password.
// It verifies the current password before applying the change.
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")
	uid, ok := userID.(string)
	if !ok || uid == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	var req passwordChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	user, err := h.DB.GetUser(uid)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve user for password change", err)
		return
	}

	if !verifyPassword(req.CurrentPassword, user.PasswordHash) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}

	hashed, err := hashPassword(req.NewPassword)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to hash password", err)
		return
	}
	user.PasswordHash = hashed
	if err := h.DB.UpdateUser(user); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update password", err)
		return
	}

	// Revoke all refresh tokens so user must re-login.
	_ = h.DB.RevokeAllUserTokens(uid)

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "system", uid, "Changed own password")
	}

	c.JSON(http.StatusOK, gin.H{"message": "password changed"})
}

// Unlock clears the account lockout for a user. Admin only.
//
//	POST /api/nvr/users/:id/unlock
func (h *UserHandler) Unlock(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")

	user, err := h.DB.GetUser(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve user", err)
		return
	}

	if err := h.DB.UnlockUser(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to unlock user", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "unlock", "user", id, "Unlocked account for user "+user.Username)
	}

	c.JSON(http.StatusOK, gin.H{"message": "account unlocked", "username": user.Username})
}

// Delete deletes a user by ID. Prevents self-deletion.
func (h *UserHandler) Delete(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")

	// Prevent self-deletion.
	currentUserID, _ := c.Get("user_id")
	if currentUserID == id {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}

	if err := h.DB.DeleteUser(id); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete user", err)
		return
	}

	// Revoke all tokens for the deleted user.
	_ = h.DB.RevokeAllUserTokens(id)

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "user", id, "Deleted user")
	}

	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}
