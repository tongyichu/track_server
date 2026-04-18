package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"trackapp-server/internal/service"
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
	userID := c.Param("user_id")
	user, err := h.userSvc.GetUserProfile(ctx, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateAvatar handles PUT /api/user/profile/photo
func (h *UserHandler) UpdateAvatar(ctx context.Context, c *app.RequestContext) {
	userID := string(c.Query("user_id"))
	var body struct {
		AvatarURL string `json:"avatar_url"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.AvatarURL == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid avatar payload"})
		return
	}
	user, err := h.userSvc.UpdateAvatar(ctx, userID, body.AvatarURL)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateName handles PUT /api/user/profile/name
func (h *UserHandler) UpdateName(ctx context.Context, c *app.RequestContext) {
	userID := string(c.Query("user_id"))
	var body struct {
		Name string `json:"name"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid name payload"})
		return
	}
	user, err := h.userSvc.UpdateName(ctx, userID, body.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateSignature handles PUT /api/user/profile/signature
func (h *UserHandler) UpdateSignature(ctx context.Context, c *app.RequestContext) {
	userID := string(c.Query("user_id"))
	var body struct {
		Signature string `json:"signature"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid signature payload"})
		return
	}
	user, err := h.userSvc.UpdateSignature(ctx, userID, body.Signature)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, user)
}

// UpdateClientLanguage handles PUT /api/user/profile/client_language
func (h *UserHandler) UpdateClientLanguage(ctx context.Context, c *app.RequestContext) {
	userID := string(c.Query("user_id"))
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
