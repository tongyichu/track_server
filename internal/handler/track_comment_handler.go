package handler

import (
	"context"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
	"strconv"
)

type CommentHandler struct{ service *service.CommentService }

func NewCommentHandler(s *service.CommentService) *CommentHandler { return &CommentHandler{s} }
func (h *CommentHandler) user(c *app.RequestContext) int64 {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(401, utils.H{"error": "unauthorized"})
		return 0
	}
	if h.service == nil {
		c.JSON(503, utils.H{"error": "comments not configured"})
		return 0
	}
	return meta.AuthUserID
}
func (h *CommentHandler) respond(c *app.RequestContext, value interface{}, err error) {
	if err == nil {
		c.JSON(200, successResponse(value))
		return
	}
	status := 500
	var invalid *service.InvalidArgumentError
	var restricted *service.AccountRestrictionBlockedError
	switch {
	case errors.Is(err, repository.ErrNotFound):
		status = 404
	case errors.Is(err, repository.ErrForbidden):
		status = 403
	case errors.Is(err, repository.ErrCommentConflict):
		status = 409
	case errors.Is(err, repository.ErrCommentUnavailable):
		status = 503
	case errors.As(err, &invalid):
		status = 400
	case errors.As(err, &restricted):
		status = 403
	}
	c.JSON(status, utils.H{"error": err.Error(), "message": err.Error()})
}
func (h *CommentHandler) List(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	limit := 20
	if raw := string(c.Query("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 {
			c.JSON(400, utils.H{"error": "invalid limit"})
			return
		}
		limit = n
	}
	page, err := h.service.Page(ctx, user, c.Param("track_id"), c.Param("comment_id"), string(c.Query("cursor")), limit)
	h.respond(c, page, err)
}
func (h *CommentHandler) Likers(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	limit := 20
	if raw := string(c.Query("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			c.JSON(400, utils.H{"error": "invalid limit"})
			return
		}
		limit = value
	}
	page, err := h.service.Likers(ctx, user, c.Param("track_id"), c.Param("comment_id"), string(c.Query("cursor")), limit)
	h.respond(c, page, err)
}
func (h *CommentHandler) Create(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	var input service.CommentInput
	if len(c.Request.Body()) > 65536 || c.BindJSON(&input) != nil {
		c.JSON(400, utils.H{"error": "invalid comment payload"})
		return
	}
	value, err := h.service.Create(ctx, user, c.Param("track_id"), input)
	h.respond(c, value, err)
}
func (h *CommentHandler) Delete(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	count, err := h.service.Delete(ctx, user, c.Param("track_id"), c.Param("comment_id"))
	h.respond(c, utils.H{"status": "ok", "comment_count": count}, err)
}
func (h *CommentHandler) React(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	var input struct {
		Reaction *int `json:"reaction"`
	}
	if c.BindJSON(&input) != nil || input.Reaction == nil {
		c.JSON(400, utils.H{"error": "reaction is required"})
		return
	}
	value, err := h.service.React(ctx, user, c.Param("track_id"), c.Param("comment_id"), *input.Reaction)
	h.respond(c, value, err)
}
func (h *CommentHandler) PrepareImage(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	var input struct {
		Width    int   `json:"width"`
		Height   int   `json:"height"`
		ByteSize int64 `json:"byte_size"`
	}
	if c.BindJSON(&input) != nil {
		c.JSON(400, utils.H{"error": "invalid image payload"})
		return
	}
	value, err := h.service.PrepareImage(ctx, user, c.Param("track_id"), input.Width, input.Height, input.ByteSize)
	h.respond(c, value, err)
}
func (h *CommentHandler) Image(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	file, err := h.service.ImageFile(ctx, user, c.Param("track_id"), c.Param("comment_id"), c.Param("image_id"))
	if err != nil {
		h.respond(c, nil, err)
		return
	}
	c.Header("Cache-Control", "private, max-age=300")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Type", "image/jpeg")
	c.File(file)
}

func (h *CommentHandler) Detail(ctx context.Context, c *app.RequestContext) {
	user := h.user(c)
	if user == 0 {
		return
	}
	value, err := h.service.Detail(ctx, user, c.Param("track_id"), c.Param("comment_id"))
	h.respond(c, value, err)
}
