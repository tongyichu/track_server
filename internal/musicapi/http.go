package musicapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
	"io"
	"time"
)

type Handler struct {
	Service *service.MusicService
	Admin   bool
}
type item struct {
	models.MusicTrack
	AudioURL string `json:"audio_url"`
	CoverURL string `json:"cover_url"`
}

func (h Handler) view(v models.MusicTrack) item {
	prefix := "/api/v1/music/"
	if h.Admin {
		prefix = "/admin/api/music/"
	}
	out := item{MusicTrack: v, AudioURL: prefix + v.ID + "/audio?v=" + v.AudioSHA}
	if v.AudioLink != "" {
		out.AudioURL = v.AudioLink
	}
	if out.AudioVersion == "" {
		out.AudioVersion = v.AudioSHA
	}
	if v.CoverSHA != "" {
		out.CoverURL = prefix + v.ID + "/cover?v=" + v.CoverSHA
	}
	return out
}
func (h Handler) List(ctx context.Context, c *app.RequestContext) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	items, err := h.Service.List(ctx, h.Admin)
	if err != nil {
		c.JSON(503, utils.H{"error": "乐库暂不可用"})
		return
	}
	out := make([]item, 0, len(items))
	for _, v := range items {
		out = append(out, h.view(v))
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"items": out}})
}
func (h Handler) Asset(ctx context.Context, c *app.RequestContext) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	file, mime, err := h.Service.Asset(ctx, c.Param("music_id"), c.Param("kind"), string(c.Query("v")), h.Admin)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.Status(404)
		} else {
			c.Status(503)
		}
		return
	}
	c.Header("Content-Type", mime)
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.File(file)
}
func (h Handler) Save(ctx context.Context, c *app.RequestContext) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	if !h.Admin {
		c.Status(403)
		return
	}
	if c.Request.Header.ContentLength() > 38*1024*1024 || len(c.Request.Body()) > 38*1024*1024 {
		c.JSON(413, utils.H{"error": "上传内容过大"})
		return
	}
	var v models.MusicTrack
	if err := json.Unmarshal([]byte(c.PostForm("metadata")), &v); err != nil {
		c.JSON(400, utils.H{"error": "音乐资料无效"})
		return
	}
	if id := c.Param("music_id"); id != "" {
		if v.ID != id {
			c.Status(400)
			return
		}
	} else if v.ID != "" {
		c.Status(400)
		return
	}
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(400, utils.H{"error": "invalid multipart upload"})
		return
	}
	read := func(name string, limit int64) ([]byte, error) {
		files := form.File[name]
		if len(files) == 0 {
			return nil, nil
		}
		if len(files) != 1 {
			return nil, service.ErrMusicInput
		}
		f := files[0]
		if f.Size > limit {
			return nil, service.ErrMusicInput
		}
		r, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer r.Close()
		data, err := io.ReadAll(io.LimitReader(r, limit+1))
		if int64(len(data)) > limit {
			return nil, service.ErrMusicInput
		}
		return data, err
	}
	audio, err := read("audio", 30*1024*1024)
	if err != nil {
		c.JSON(400, utils.H{"error": "音频上传无效或大于30MB"})
		return
	}
	cover, err := read("cover", 5*1024*1024)
	if err != nil {
		c.JSON(400, utils.H{"error": "封面上传无效或大于5MB"})
		return
	}
	saved, err := h.Service.Save(ctx, v, audio, cover)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrMusicInput):
			c.JSON(400, utils.H{"error": err.Error()})
		case errors.Is(err, repository.ErrMusicConflict):
			c.JSON(409, utils.H{"error": "音乐已被修改，请刷新后重试"})
		case errors.Is(err, repository.ErrNotFound):
			c.Status(404)
		default:
			c.JSON(503, utils.H{"error": "音乐保存失败，请重试"})
		}
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": h.view(saved)})
}

func (h Handler) Arrange(ctx context.Context, c *app.RequestContext) {
	if !h.Admin {
		c.Status(403)
		return
	}
	c.Header("Cache-Control", "no-store")
	if len(c.Request.Body()) > 96*1024 {
		c.Status(413)
		return
	}
	var input struct {
		Items []service.MusicPlacement `json:"items"`
	}
	if err := json.Unmarshal(c.Request.Body(), &input); err != nil || input.Items == nil {
		c.JSON(400, utils.H{"error": "音乐布局无效"})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	items, saved, err := h.Service.Arrange(ctx, input.Items)
	if err != nil {
		status, message := 503, "布局保存失败，请刷新确认当前顺序后重试"
		if errors.Is(err, repository.ErrMusicConflict) {
			status, message = 409, "乐库已被修改，请刷新后重新调整"
		}
		if errors.Is(err, service.ErrMusicInput) {
			status, message = 400, err.Error()
		}
		c.JSON(status, utils.H{"error": message, "saved_count": saved})
		return
	}
	out := make([]item, 0, len(items))
	for _, v := range items {
		out = append(out, h.view(v))
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"items": out}})
}

func (h Handler) Delete(ctx context.Context, c *app.RequestContext) {
	if !h.Admin {
		c.Status(403)
		return
	}
	c.Header("Cache-Control", "no-store")
	if len(c.Request.Body()) > 1024 {
		c.Status(413)
		return
	}
	var input struct {
		Revision int64 `json:"revision"`
	}
	if err := json.Unmarshal(c.Request.Body(), &input); err != nil || input.Revision < 1 {
		c.JSON(400, utils.H{"error": "音乐版本无效"})
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := h.Service.Delete(ctx, c.Param("music_id"), input.Revision); err != nil {
		if errors.Is(err, repository.ErrMusicConflict) {
			c.JSON(409, utils.H{"error": "音乐已被修改或删除，请刷新后重试"})
		} else {
			c.JSON(503, utils.H{"error": "音乐删除失败，请刷新确认结果后重试"})
		}
		return
	}
	c.JSON(200, utils.H{"code": 0, "data": utils.H{"deleted": true}})
}
