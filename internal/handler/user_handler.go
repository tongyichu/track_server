package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/service"
)

// UserHandler handles HTTP requests related to user profile and settings.
type UserHandler struct {
	userSvc *service.UserService
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(userSvc *service.UserService) *UserHandler {
	return &UserHandler{userSvc: userSvc}
}

// GetUserDetail handles GET /api/user/:user_id/detail
func (h *UserHandler) GetUserDetail(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	authUserID := meta.AuthUserID

	userID, err := parseRequiredUserID(c.Param("user_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	if userID != authUserID {
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
		return
	}
	user, err := h.userSvc.GetUserProfile(ctx, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
		return
	}
	stats, err := h.userSvc.GetUserStats(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	data := struct {
		*models.User
		TotalDistance  float64 `json:"total_distance"`
		TrackCount     int64   `json:"track_count"`
		TrackUsedCount int64   `json:"track_used_count"`
	}{
		User:           user,
		TotalDistance:  stats.TotalDistance,
		TrackCount:     stats.TrackCount,
		TrackUsedCount: stats.TrackUsedCount,
	}
	c.JSON(http.StatusOK, successResponse(data))
}

// UpdateProfile handles PUT /api/v1/user/profile/update
// It merges UpdateAvatar/UpdateName/UpdateSignature and only allows updating current authorized user.
func (h *UserHandler) UpdateProfile(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	authUserID := meta.AuthUserID

	var body struct {
		AvatarURL  *string `json:"avatar_url"`
		Name       *string `json:"name"`
		Signature  *string `json:"signature"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}

	user, err := h.userSvc.UpdateProfile(ctx, authUserID, service.UserProfilePatch{
		AvatarURL: body.AvatarURL,
		Name:      body.Name,
		Signature: body.Signature,
	})
	if err != nil {
		// Keep consistent with existing handler error style.
		if errors.Is(err, service.ErrNoFieldsToUpdate) || errors.Is(err, service.ErrAvatarURLRequired) || errors.Is(err, service.ErrNameRequired) {
			c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	// StandardResponse
	c.JSON(http.StatusOK, successResponse(user))
}

// UpdatePhone handles PUT /api/user/profile/phone
func (h *UserHandler) UpdatePhone(ctx context.Context, c *app.RequestContext) {
	userID, err := parseRequiredUserID(string(c.Query("user_id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	var body struct {
		Phone string `json:"phone"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.Phone == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid phone payload"})
		return
	}
	user, err := h.userSvc.UpdatePhone(ctx, userID, body.Phone)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateClientLanguage handles PUT /api/user/profile/client_language
func (h *UserHandler) UpdateClientLanguage(ctx context.Context, c *app.RequestContext) {
	userID, err := parseRequiredUserID(string(c.Query("user_id")))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	var body struct {
		ClientLanguage string `json:"client_language"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.ClientLanguage == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid language payload"})
		return
	}
	user, err := h.userSvc.UpdateClientLanguage(ctx, userID, body.ClientLanguage)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

func parseRequiredUserID(raw string) (int64, error) {
	if raw == "" {
		return 0, errors.New("user_id is required")
	}
	userID, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || userID <= 0 {
		return 0, errors.New("user_id must be int64")
	}
	return userID, nil
}
