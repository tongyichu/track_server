package admin

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

type adminUserProfileResponse struct {
	ID           string    `json:"id"`
	Nickname     string    `json:"nickname"`
	AvatarURL    string    `json:"avatar_url"`
	Signature    string    `json:"signature"`
	Phone        string    `json:"phone"`
	Gender       string    `json:"gender"`
	Birthday     string    `json:"birthday"`
	ProvinceCode string    `json:"province_code"`
	ProvinceName string    `json:"province_name"`
	CityCode     string    `json:"city_code"`
	CityName     string    `json:"city_name"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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
		Nickname  string `json:"nickname"`
		Signature string `json:"signature"`
		Phone     string `json:"phone"`
		Gender    string `json:"gender"`
		Birthday  string `json:"birthday"`
	}
	data, err := c.Body()
	if err != nil || json.Unmarshal(data, &body) != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	body.Nickname = strings.TrimSpace(body.Nickname)
	body.Phone = strings.TrimSpace(body.Phone)
	body.Gender = strings.TrimSpace(body.Gender)
	body.Birthday = strings.TrimSpace(body.Birthday)
	if body.Nickname == "" || utf8.RuneCountInString(body.Nickname) > 20 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "nickname must contain 1-20 characters"})
		return
	}
	if utf8.RuneCountInString(body.Signature) > 100 || utf8.RuneCountInString(body.Phone) > 32 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "signature or phone is too long"})
		return
	}
	if body.Gender != "" && body.Gender != "male" && body.Gender != "female" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "gender must be empty, male or female"})
		return
	}
	if body.Birthday != "" {
		birthday, err := time.Parse(time.DateOnly, body.Birthday)
		if err != nil || birthday.Format(time.DateOnly) != body.Birthday || body.Birthday < "1900-01-01" || body.Birthday > time.Now().Format(time.DateOnly) {
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
	clone.Phone = body.Phone
	clone.Gender = body.Gender
	clone.Birthday = body.Birthday
	if err := h.userRepo.Update(ctx, &clone); err != nil {
		writeAdminUserProfileError(c, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"code": 0, "data": h.adminUserProfile(ctx, &clone)})
}

func (h *Handler) adminUserProfile(ctx context.Context, user *models.User) *adminUserProfileResponse {
	if h.userSvc != nil {
		h.userSvc.DecorateAvatar(ctx, user)
	}
	return &adminUserProfileResponse{
		ID: strconv.FormatInt(user.ID, 10), Nickname: user.Nickname, AvatarURL: rewriteAdminStaticURL(user.AvatarURL),
		Signature: user.Signature, Phone: user.Phone, Gender: user.Gender, Birthday: user.Birthday,
		ProvinceCode: user.ProvinceCode, ProvinceName: user.ProvinceName,
		CityCode: user.CityCode, CityName: user.CityName,
		CreatedAt: user.CreatedAt, UpdatedAt: user.UpdatedAt,
	}
}

func writeAdminUserProfileError(c *app.RequestContext, err error) {
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
}
