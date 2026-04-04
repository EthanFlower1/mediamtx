package api

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

type BookmarkHandler struct {
	DB *db.DB
}

type CreateBookmarkRequest struct {
	CameraID  string `json:"camera_id" binding:"required"`
	Timestamp string `json:"timestamp" binding:"required"`
	Label     string `json:"label" binding:"required"`
	Notes     string `json:"notes"`
}

func (h *BookmarkHandler) Create(c *gin.Context) {
	var req CreateBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id, timestamp, and label are required"})
		return
	}

	if !hasCameraPermission(c, req.CameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	b := &db.Bookmark{
		CameraID:  req.CameraID,
		Timestamp: req.Timestamp,
		Label:     req.Label,
		Notes:     req.Notes,
		CreatedBy: username,
	}

	if err := h.DB.InsertBookmark(b); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create bookmark", err)
		return
	}

	c.JSON(http.StatusCreated, b)
}

func (h *BookmarkHandler) List(c *gin.Context) {
	cameraID := c.Query("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	dateStr := c.Query("date")
	date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date"})
		return
	}

	bookmarks, err := h.DB.GetBookmarks(cameraID, date, date.Add(24*time.Hour))
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query bookmarks", err)
		return
	}

	if bookmarks == nil {
		bookmarks = []db.Bookmark{}
	}

	c.JSON(http.StatusOK, bookmarks)
}

type UpdateBookmarkRequest struct {
	Label string `json:"label" binding:"required"`
	Notes string `json:"notes"`
}

func (h *BookmarkHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}

	var req UpdateBookmarkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "label is required"})
		return
	}

	if err := h.DB.UpdateBookmark(id, req.Label, req.Notes); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update bookmark", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *BookmarkHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}

	if err := h.DB.DeleteBookmark(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete bookmark", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *BookmarkHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
		return
	}

	b, err := h.DB.GetBookmark(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get bookmark", err)
		return
	}

	c.JSON(http.StatusOK, b)
}

func (h *BookmarkHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "q parameter is required"})
		return
	}

	bookmarks, err := h.DB.SearchBookmarks(query)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to search bookmarks", err)
		return
	}

	if bookmarks == nil {
		bookmarks = []db.Bookmark{}
	}

	c.JSON(http.StatusOK, bookmarks)
}

func (h *BookmarkHandler) Mine(c *gin.Context) {
	usernameVal, _ := c.Get("username")
	username, _ := usernameVal.(string)
	if username == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "not authenticated"})
		return
	}

	bookmarks, err := h.DB.GetBookmarksByUser(username)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get bookmarks", err)
		return
	}

	if bookmarks == nil {
		bookmarks = []db.Bookmark{}
	}

	c.JSON(http.StatusOK, bookmarks)
}
