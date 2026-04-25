package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"

	"github.com/tongyichu/track_server/internal/middleware"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

// Ping handles GET /api/ping
func Ping(ctx context.Context, c *app.RequestContext) {
	c.String(http.StatusOK, "hello track app")
}

// TrackHandler handles HTTP requests related to tracks.
type TrackHandler struct {
	trackSvc *service.TrackService
}

type RunningTrackResult struct {
	Running bool          `json:"running"`
	Track   *models.Track `json:"track,omitempty"`
}

type StatusResult struct {
	Status string `json:"status"`
}

// NewTrackHandler creates a new TrackHandler.
func NewTrackHandler(trackSvc *service.TrackService) *TrackHandler {
	return &TrackHandler{trackSvc: trackSvc}
}

// CreateTrack handles POST /api/track/create
func (h *TrackHandler) CreateTrack(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}

	var req service.CreateTrackInput
	data, err := c.Body()
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &req); err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
			return
		}
	}

	track, err := h.trackSvc.CreateTrack(ctx, meta.AuthUserID, req)
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(track))
}

// UpdateTrackInfo handles PUT /api/v1/track/:track_id/update
// It supports partial update of: Distance, Duration, ElevationGain, RawTrackURL,
// TrackScreenshotURL, IsRunning, AvgSpeedKmh.
func (h *TrackHandler) UpdateTrackInfo(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}

	trackID := c.Param("track_id")
	var patch service.TrackInfoPatch
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &patch) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}

	track, err := h.trackSvc.UpdateTrackInfo(ctx, meta.AuthUserID, trackID, patch)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrNotFound):
			c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
			return
		case errors.Is(err, service.ErrForbidden):
			c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
			return
		default:
			var iae *service.InvalidArgumentError
			if errors.As(err, &iae) {
				c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
				return
			}
			c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
			return
		}
	}
	if track == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "unexpected nil track"})
		return
	}
	c.JSON(http.StatusOK, successResponse(track))
}

// ListRecommend handles GET /api/track/recommend/list
func (h *TrackHandler) ListRecommend(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	var userID int64
	if meta != nil {
		if meta.RawUserID != "" && meta.AuthUserID <= 0 {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
			return
		}
		userID = meta.AuthUserID
	}
	tracks, err := h.trackSvc.ListRecommend(ctx, userID, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(tracks))
}

// ListMyTracks handles GET /api/v1/track/my/list
func (h *TrackHandler) ListMyTracks(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	tracks, err := h.trackSvc.ListMyTracks(ctx, meta.AuthUserID, 50)
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(tracks))
}

// SearchTracks handles GET /api/track/search/list
func (h *TrackHandler) SearchTracks(ctx context.Context, c *app.RequestContext) {
	// 说明：
	// - 该接口会在返回 TrackSummary 时同时补齐 collected/collect_count 以及用户昵称/头像等字段。
	// - collected 的计算依赖“当前鉴权用户”，因此这里需要从请求上下文解析出 AuthUserID 并透传到 service。
	meta := middleware.GetRequestMeta(c)
	var userID int64
	if meta != nil {
		if meta.RawUserID != "" && meta.AuthUserID <= 0 {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
			return
		}
		userID = meta.AuthUserID
	}
	keyword := string(c.Query("keyword"))
	tracks, err := h.trackSvc.SearchTracks(ctx, userID, keyword, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tracks)
}

// GetTrackMap handles GET /api/track/:track_id/map
func (h *TrackHandler) GetTrackMap(ctx context.Context, c *app.RequestContext) {
	trackID := c.Param("track_id")
	m, err := h.trackSvc.GetTrackMap(ctx, trackID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == repository.ErrNotFound.Error() {
			status = http.StatusNotFound
		}
		c.JSON(status, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, m)
}

// ReportTrackNavigation handles POST /api/track/:track_id/navigation/report
func (h *TrackHandler) ReportTrackNavigation(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	trackID := c.Param("track_id")
	if err := h.trackSvc.ReportTrackNavigation(ctx, meta.AuthUserID, trackID); err != nil {
		status := http.StatusInternalServerError
		switch {
		case err.Error() == repository.ErrNotFound.Error():
			status = http.StatusNotFound
		case errors.As(err, new(*service.InvalidArgumentError)):
			status = http.StatusBadRequest
		}
		c.JSON(status, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(utils.H{"status": "ok"}))
}

// GetTrackDetail handles GET /api/track/:track_id/detail
func (h *TrackHandler) GetTrackDetail(ctx context.Context, c *app.RequestContext) {
	trackID := c.Param("track_id")
	track, err := h.trackSvc.GetTrackDetail(ctx, trackID)
	if err != nil {
		status := http.StatusInternalServerError
		if err.Error() == repository.ErrNotFound.Error() {
			status = http.StatusNotFound
		}
		c.JSON(status, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(track))
}

// UploadTrackCloud handles POST /api/track/:track_id/upload_cloud
func (h *TrackHandler) UploadTrackCloud(ctx context.Context, c *app.RequestContext) {
	trackID := c.Param("track_id")
	if err := h.trackSvc.MarkUploadedToCloud(ctx, trackID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == repository.ErrNotFound.Error() {
			status = http.StatusNotFound
		}
		c.JSON(status, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"status": "ok"})
}

// GetRunningTrack handles GET /api/track/running
func (h *TrackHandler) GetRunningTrack(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	authUserID := meta.AuthUserID

	rawHeaderUserID := string(c.Request.Header.Peek(middleware.HeaderUserID))
	if rawHeaderUserID != "" {
		headerUserID, err := parseRequiredUserID(rawHeaderUserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid X-User-ID header"})
			return
		}
		if headerUserID != authUserID {
			c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
			return
		}
	}

	rawUserID := string(c.Query("user_id"))
	userID := authUserID
	if rawUserID != "" {
		parsedUserID, err := parseRequiredUserID(rawUserID)
		if err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
			return
		}
		if parsedUserID != authUserID {
			c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
			return
		}
		userID = parsedUserID
	}

	track, err := h.trackSvc.GetRunningTrack(ctx, userID)
	if err != nil {
		if err.Error() == repository.ErrNotFound.Error() {
			c.JSON(http.StatusOK, successResponse(RunningTrackResult{Running: false}))
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(RunningTrackResult{Running: true, Track: track}))
}

// GetCollectStatus handles GET /api/user/:user_id/collect
func (h *TrackHandler) GetCollectStatus(ctx context.Context, c *app.RequestContext) {
	userID, err := parseRequiredUserID(c.Param("user_id"))
	trackID := string(c.Query("track_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": err.Error()})
		return
	}
	if trackID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "user_id and track_id are required"})
		return
	}
	collected, err := h.trackSvc.IsCollected(ctx, userID, trackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"collected": collected})
}

// CollectTrack handles POST /api/track_collect
func (h *TrackHandler) CollectTrack(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	userID := meta.AuthUserID
	trackID := string(c.Query("track_id"))
	if trackID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "track_id is required"})
		return
	}
	if err := h.trackSvc.CollectTrack(ctx, userID, trackID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == repository.ErrNotFound.Error() {
			status = http.StatusNotFound
		}
		c.JSON(status, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}

// UncollectTrack handles DELETE /api/track_collect
func (h *TrackHandler) UncollectTrack(ctx context.Context, c *app.RequestContext) {
	meta := middleware.GetRequestMeta(c)
	if meta == nil || meta.AuthUserID <= 0 {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
		return
	}
	userID := meta.AuthUserID
	trackID := string(c.Query("track_id"))
	if trackID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "track_id is required"})
		return
	}
	if err := h.trackSvc.UncollectTrack(ctx, userID, trackID); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(StatusResult{Status: "ok"}))
}
