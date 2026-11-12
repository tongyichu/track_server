package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

type adminUserProfileResponse struct {
	ID                   string     `json:"id"`
	Nickname             string     `json:"nickname"`
	AvatarURL            string     `json:"avatar_url"`
	Signature            string     `json:"signature"`
	BackgroundURL        string     `json:"background_url"`
	ZodiacSign           string     `json:"zodiac_sign"`
	Phone                string     `json:"phone"`
	Gender               string     `json:"gender"`
	Birthday             string     `json:"birthday"`
	ProvinceCode         string     `json:"province_code"`
	ProvinceName         string     `json:"province_name"`
	CityCode             string     `json:"city_code"`
	CityName             string     `json:"city_name"`
	IsInternal           bool       `json:"is_internal"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	LastActiveAt         *time.Time `json:"last_active_at"`
	LastActiveAppVersion *string    `json:"last_active_app_version"`
	ActiveDays7D         *int       `json:"active_days_7d"`
}

func (h *Handler) GetUserProfile(ctx context.Context, c *app.RequestContext) {
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	user, err := h.userRepo.FindByID(ctx, userID)
	if err != nil {
		writeAdminUserProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": h.adminUserProfile(ctx, user)})
}

func (h *Handler) UpdateUserProfile(ctx context.Context, c *app.RequestContext) {
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	var body struct {
		Nickname   string          `json:"nickname"`
		Signature  string          `json:"signature"`
		Phone      json.RawMessage `json:"phone"`
		Gender     string          `json:"gender"`
		Birthday   string          `json:"birthday"`
		IsInternal bool            `json:"is_internal"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if len(body.Phone) != 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "手机号为只读，不能在后台修改"})
		return
	}
	body.Nickname = strings.TrimSpace(body.Nickname)
	body.Gender = strings.TrimSpace(body.Gender)
	body.Birthday = strings.TrimSpace(body.Birthday)
	if body.Nickname == "" || utf8.RuneCountInString(body.Nickname) > 20 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "nickname must contain 1-20 characters"})
		return
	}
	if utf8.RuneCountInString(body.Signature) > 100 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "signature is too long"})
		return
	}
	if body.Gender != "" && body.Gender != "male" && body.Gender != "female" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "gender must be empty, male or female"})
		return
	}
	if body.Birthday != "" {
		birthday, err := time.Parse(time.DateOnly, body.Birthday)
		if err != nil || birthday.Format(time.DateOnly) != body.Birthday || body.Birthday < "1900-01-01" || body.Birthday > time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format(time.DateOnly) {
			c.JSON(http.StatusBadRequest, utils.H{"error": "invalid birthday"})
			return
		}
	}
	user, err := h.userRepo.FindByID(ctx, userID)
	if err != nil {
		writeAdminUserProfileError(c, err)
		return
	}
	clone := *user
	clone.Nickname = body.Nickname
	clone.Signature = body.Signature
	clone.Gender = body.Gender
	clone.Birthday = body.Birthday
	clone.IsInternal = body.IsInternal
	if err := h.userRepo.Update(ctx, &clone); err != nil {
		writeAdminUserProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": h.adminUserProfile(ctx, &clone)})
}

func (h *Handler) UpdateUserInternal(ctx context.Context, c *app.RequestContext) {
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	var body struct {
		IsInternal bool `json:"is_internal"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	if err := h.userRepo.UpdateInternal(ctx, userID, body.IsInternal); err != nil {
		writeAdminUserProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": utils.H{"status": "ok", "is_internal": body.IsInternal}})
}

func (h *Handler) adminUserProfile(ctx context.Context, user *models.User) *adminUserProfileResponse {
	copy := *user
	user = &copy
	if h.userSvc != nil {
		h.userSvc.DecorateAvatar(ctx, user)
	}
	backgroundURL := ""
	if strings.TrimSpace(user.BackgroundURL) != "" {
		backgroundURL = "/admin/api/users/" + strconv.FormatInt(user.ID, 10) + "/background"
	}
	var lastActiveAt *time.Time
	var lastActiveAppVersion *string
	var activeDays7D *int
	if stats, ok := h.adminUserActivityStats(ctx, []int64{user.ID})[user.ID]; ok {
		lastActiveAt = stats.LastActiveAt
		lastActiveAppVersion = adminUserActivityVersion(stats)
		activeDays7D = &stats.ActiveDays7D
	}
	return &adminUserProfileResponse{
		ID: strconv.FormatInt(user.ID, 10), Nickname: user.Nickname, AvatarURL: rewriteAdminStaticURL(user.AvatarURL),
		BackgroundURL: backgroundURL, ZodiacSign: service.ZodiacSignFromBirthday(user.Birthday),
		Signature: user.Signature, Phone: user.Phone, Gender: user.Gender, Birthday: user.Birthday,
		ProvinceCode: user.ProvinceCode, ProvinceName: user.ProvinceName,
		CityCode: user.CityCode, CityName: user.CityName, IsInternal: user.IsInternal,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
		LastActiveAt:         lastActiveAt,
		LastActiveAppVersion: lastActiveAppVersion,
		ActiveDays7D:         activeDays7D,
	}
}

func adminUserActivityVersion(stats models.UserActivityStats) *string {
	if stats.LastActiveAt == nil || stats.LastActiveAt.IsZero() || strings.TrimSpace(stats.LastActiveAppVersion) == "" {
		return nil
	}
	return &stats.LastActiveAppVersion
}

func (h *Handler) adminUserActivityStats(ctx context.Context, userIDs []int64) map[int64]models.UserActivityStats {
	repo, ok := h.userRepo.(repository.UserActivityRepository)
	if !ok || len(userIDs) == 0 {
		return nil
	}
	readCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	values, err := repo.GetUserActivityStats(readCtx, userIDs, time.Now())
	if err != nil {
		log.Printf("admin user activity unavailable: %v", err)
		return nil
	}
	return values
}

func writeAdminUserProfileError(c *app.RequestContext, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
}

// Overview and media load independently so profile fields never wait on them.
func (h *Handler) GetUserProfileOverview(ctx context.Context, c *app.RequestContext) {
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	if _, err := h.userRepo.FindByID(ctx, userID); err != nil {
		writeAdminUserProfileError(c, err)
		return
	}
	overview, err := h.userSvc.GetUserProfileOverview(ctx, userID)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "互动数据与级别暂不可用，请重试"})
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": overview})
}

func (h *Handler) GetUserProfileBackground(ctx context.Context, c *app.RequestContext) {
	userID, ok := parseAdminUserID(c)
	if !ok {
		return
	}
	if h.userSvc == nil || h.staticRoot == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "背景图服务暂不可用"})
		return
	}
	user, err := h.userSvc.GetUserProfile(ctx, userID)
	if err != nil {
		writeAdminUserProfileError(c, err)
		return
	}
	if user.BackgroundURL == "" {
		c.JSON(http.StatusNotFound, utils.H{"error": "未设置或暂时无法读取背景图"})
		return
	}
	const prefix = "/api/v1/static/profile_backgrounds/"
	if !strings.HasPrefix(user.BackgroundURL, prefix) {
		c.JSON(http.StatusBadGateway, utils.H{"error": "背景图暂不可用"})
		return
	}
	rel, valid := cleanAdminStaticPath(strings.TrimPrefix(user.BackgroundURL, "/api/v1/static/"))
	if !valid {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid background path"})
		return
	}
	data, err := os.ReadFile(filepath.Join(h.staticRoot, filepath.FromSlash(rel)))
	if err != nil {
		c.JSON(http.StatusNotFound, utils.H{"error": "背景图暂不可用"})
		return
	}
	c.Response.Header.Set("Cache-Control", "no-store")
	c.Response.Header.Set("X-Content-Type-Options", "nosniff")
	c.Response.Header.Set("Content-Security-Policy", "default-src 'none'; sandbox")
	c.Data(http.StatusOK, contentTypeFor(rel), data)
}
