package admin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

func (h *Handler) scenicSpotServiceError(c *app.RequestContext, err error) {
	var invalid *service.InvalidArgumentError
	switch {
	case errors.As(err, &invalid):
		c.JSON(http.StatusBadRequest, utils.H{"error": invalid.Error()})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, utils.H{"error": "not found"})
	case errors.Is(err, repository.ErrConflict):
		c.JSON(http.StatusConflict, utils.H{"error": "version or assignment conflict"})
	case errors.Is(err, repository.ErrAlreadyExists):
		c.JSON(http.StatusConflict, utils.H{"error": "provider identity or id already exists"})
	case errors.Is(err, repository.ErrForbidden):
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
	case strings.Contains(err.Error(), "provider verification") || strings.Contains(err.Error(), "provider unavailable") || strings.Contains(err.Error(), "place id") || strings.Contains(err.Error(), "place source"):
		c.JSON(http.StatusBadGateway, utils.H{"error": "provider verification failed"})
	default:
		c.JSON(http.StatusInternalServerError, utils.H{"error": "scenic spot operation failed"})
	}
}
func (h *Handler) scenicSpotServiceReady(c *app.RequestContext) bool {
	if h == nil || h.scenicSpotSvc == nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "scenic spot service unavailable"})
		return false
	}
	return true
}

func (h *Handler) ListScenicSpots(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	f := repository.ScenicSpotListFilter{Query: strings.TrimSpace(string(c.Query("q"))), Status: strings.TrimSpace(string(c.Query("status"))), Cursor: strings.TrimSpace(string(c.Query("cursor"))), Limit: parseAdminListLimit(string(c.Query("limit")))}
	items, total, err := h.scenicSpotSvc.List(ctx, f)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	next := ""
	if len(items) > 0 {
		next = items[len(items)-1].ID
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"items": items, "total": total, "next_cursor": next, "has_more": len(items) == f.Limit}})
}
func (h *Handler) GetScenicSpotStatistics(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	stats, err := h.scenicSpotSvc.Statistics(ctx)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": stats})
}
func (h *Handler) GetScenicSpot(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	v, err := h.scenicSpotSvc.Get(ctx, c.Param("spot_id"))
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": v})
}
func (h *Handler) CreateScenicSpot(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body models.ScenicSpot
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	v, err := h.scenicSpotSvc.Create(ctx, h.adminOperator(c), &body)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": v})
}
func (h *Handler) UpdateScenicSpot(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body struct {
		ExpectedVersion int64             `json:"expected_version"`
		Spot            models.ScenicSpot `json:"spot"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || body.ExpectedVersion <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "expected_version and spot are required"})
		return
	}
	v, err := h.scenicSpotSvc.Update(ctx, h.adminOperator(c), c.Param("spot_id"), body.ExpectedVersion, &body.Spot)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": v})
}
func (h *Handler) SetScenicSpotStatus(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body struct {
		Status          string `json:"status"`
		ExpectedVersion int64  `json:"expected_version"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || body.ExpectedVersion <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	v, err := h.scenicSpotSvc.SetStatus(ctx, h.adminOperator(c), c.Param("spot_id"), body.Status, body.ExpectedVersion)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": v})
}
func (h *Handler) VerifyScenicSpotSource(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body struct {
		Provider   string `json:"provider"`
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || strings.TrimSpace(body.ProviderID) == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "provider and provider_id are required"})
		return
	}
	spot, err := h.scenicSpotSvc.VerifySource(ctx, h.adminOperator(c), c.Param("spot_id"), body.Provider, body.ProviderID)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": spot})
}

func (h *Handler) PreviewScenicSpotSource(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body struct {
		Provider   string `json:"provider"`
		ProviderID string `json:"provider_id"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || strings.TrimSpace(body.ProviderID) == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "provider and provider_id are required"})
		return
	}
	preview, err := h.scenicSpotSvc.PreviewProviderSource(ctx, c.Param("spot_id"), body.Provider, body.ProviderID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			h.scenicSpotServiceError(c, err)
			return
		}
		c.JSON(http.StatusBadGateway, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": preview})
}

func (h *Handler) ListScenicSpotTracks(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	id := c.Param("spot_id")
	links, err := h.scenicSpotSvc.TrackLinks(ctx, id, strings.TrimSpace(string(c.Query("cursor"))), parseAdminListLimit(string(c.Query("limit"))))
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	ids := make([]string, 0, len(links))
	for _, v := range links {
		ids = append(ids, v.TrackID)
	}
	tracks, err := h.trackRepo.FindByIDs(ctx, ids)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	items := make([]utils.H, 0, len(links))
	for _, link := range links {
		if t := tracks[link.TrackID]; t != nil {
			items = append(items, utils.H{"track_id": link.TrackID, "title": t.Title, "city_code": t.CityCode, "start_time": t.StartTime, "origin": link.Origin, "confidence": link.Confidence})
		}
	}
	next := ""
	if len(links) > 0 {
		next = links[len(links)-1].TrackID
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"items": items, "next_cursor": next, "has_more": len(links) == parseAdminListLimit(string(c.Query("limit")))}})
}
func (h *Handler) AssignScenicSpotTrack(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body struct {
		TrackID string `json:"track_id"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || strings.TrimSpace(body.TrackID) == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "track_id is required"})
		return
	}
	if err := h.scenicSpotSvc.AssignTrack(ctx, h.adminOperator(c), c.Param("spot_id"), strings.TrimSpace(body.TrackID), "manual"); err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "assigned"}})
}
func (h *Handler) UnassignScenicSpotTrack(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	if err := h.scenicSpotSvc.UnassignTrack(ctx, h.adminOperator(c), c.Param("track_id")); err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "unassigned"}})
}
func (h *Handler) MergeScenicSpots(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body struct {
		SourceID string `json:"source_id"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil || body.SourceID == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "source_id is required"})
		return
	}
	v, err := h.scenicSpotSvc.Merge(ctx, h.adminOperator(c), c.Param("spot_id"), body.SourceID)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": v})
}
func (h *Handler) SplitScenicSpot(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var body struct {
		Spot     models.ScenicSpot `json:"spot"`
		TrackIDs []string          `json:"track_ids"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	v, err := h.scenicSpotSvc.Split(ctx, h.adminOperator(c), c.Param("spot_id"), &body.Spot, body.TrackIDs)
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": v})
}
func (h *Handler) ListScenicSpotAudit(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	items, err := h.scenicSpotSvc.Audit(ctx, c.Param("spot_id"), strings.TrimSpace(string(c.Query("cursor"))), parseAdminListLimit(string(c.Query("limit"))))
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	next := ""
	if len(items) > 0 {
		next = items[len(items)-1].ID
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"items": items, "next_cursor": next, "has_more": len(items) == parseAdminListLimit(string(c.Query("limit")))}})
}
func (h *Handler) BackfillScenicSpots(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	var cursor *models.TrackListCursor
	if raw := strings.TrimSpace(string(c.Query("cursor_start_time"))); raw != "" {
		at, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil || strings.TrimSpace(string(c.Query("cursor_id"))) == "" {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid cursor"})
			return
		}
		cursor = &models.TrackListCursor{StartTime: at, ID: strings.TrimSpace(string(c.Query("cursor_id")))}
	}
	result, err := h.scenicSpotSvc.BackfillLegacy(ctx, cursor, parseAdminListLimit(string(c.Query("limit"))))
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": result})
}

func (h *Handler) SearchScenicSpotTracks(ctx context.Context, c *app.RequestContext) {
	title := strings.TrimSpace(string(c.Query("title")))
	if utf8.RuneCountInString(title) < 2 || utf8.RuneCountInString(title) > 128 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "title must be 2 to 128 characters"})
		return
	}
	search, ok := h.trackRepo.(repository.AdminTrackTitleSearchRepository)
	if !ok {
		c.JSON(http.StatusNotImplemented, utils.H{"error": "track search unavailable"})
		return
	}
	items, total, err := search.ListAdminTracksByTitle(ctx, title, nil, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "track search failed"})
		return
	}
	out := make([]utils.H, 0, len(items))
	for _, track := range items {
		out = append(out, utils.H{"id": track.ID, "title": track.Title, "city_code": track.CityCode, "start_time": track.StartTime, "status": track.Status})
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"items": out, "total": total}})
}

const maxScenicSpotPhotoSize = 8 << 20

func (h *Handler) UploadScenicSpotPhoto(ctx context.Context, c *app.RequestContext) {
	if !h.scenicSpotServiceReady(c) {
		return
	}
	if strings.TrimSpace(h.staticRoot) == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "static root not configured"})
		return
	}
	spot, err := h.scenicSpotSvc.Get(ctx, c.Param("spot_id"))
	if err != nil {
		h.scenicSpotServiceError(c, err)
		return
	}
	file, err := c.FormFile("file")
	if err != nil || file == nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "file is required"})
		return
	}
	if file.Size <= 0 || file.Size > maxScenicSpotPhotoSize {
		c.JSON(http.StatusRequestEntityTooLarge, utils.H{"error": "photo must be between 1 byte and 8 MiB"})
		return
	}
	src, err := file.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "cannot open image"})
		return
	}
	defer src.Close()
	raw, err := io.ReadAll(io.LimitReader(src, maxScenicSpotPhotoSize+1))
	if err != nil || len(raw) > maxScenicSpotPhotoSize {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid image size"})
		return
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || config.Width <= 0 || config.Height <= 0 || config.Width > 12000 || config.Height > 12000 || (format != "jpeg" && format != "png") {
		c.JSON(http.StatusBadRequest, utils.H{"error": "only valid JPEG or PNG images up to 12000x12000 are supported"})
		return
	}
	var idbuf [12]byte
	if _, err := rand.Read(idbuf[:]); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "photo id generation failed"})
		return
	}
	photoID := hex.EncodeToString(idbuf[:])
	ext := ".jpg"
	mime := "image/jpeg"
	if format == "png" {
		ext = ".png"
		mime = "image/png"
	}
	name := photoID + ext
	rel := filepath.ToSlash(filepath.Join("scenic_spots", spot.ID, name))
	dir := filepath.Join(h.staticRoot, "scenic_spots", spot.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "photo storage unavailable"})
		return
	}
	dstPath := filepath.Join(dir, name)
	if err := os.WriteFile(dstPath, raw, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "photo write failed"})
		return
	}
	photo := models.ScenicSpotPhoto{ID: photoID, URL: "/api/v1/static/" + rel, Caption: strings.TrimSpace(string(c.FormValue("caption"))), AltText: strings.TrimSpace(string(c.FormValue("alt_text"))), Attribution: strings.TrimSpace(string(c.FormValue("attribution")))}
	spot.Photos = append(spot.Photos, photo)
	expected := spot.Version
	updated, err := h.scenicSpotSvc.Update(ctx, h.adminOperator(c), spot.ID, expected, spot)
	if err != nil {
		_ = os.Remove(dstPath)
		h.scenicSpotServiceError(c, err)
		return
	}
	c.JSON(http.StatusCreated, utils.H{"code": 0, "data": utils.H{"photo": photo, "spot": updated, "content_type": mime, "size": len(raw)}})
}
