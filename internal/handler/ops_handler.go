package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

type OpsHandler struct {
	userSvc        *service.UserService
	achievementSvc *service.AchievementService
	restrictionSvc *service.AccountRestrictionService
	internalToken  string
}

type OpsAchievementRefreshResult struct {
	UserID              int64                           `json:"user_id"`
	Phone               string                          `json:"phone"`
	NewRewardCount      int                             `json:"new_reward_count"`
	EarnedRewardCount   int                             `json:"earned_reward_count"`
	QualifiedTrackCount int64                           `json:"qualified_track_count"`
	TotalXP             int64                           `json:"total_xp"`
	CurrentLevel        models.AchievementLevel         `json:"current_level"`
	NextLevel           *models.AchievementLevel        `json:"next_level,omitempty"`
	NewRewards          []*models.AchievementRewardView `json:"new_rewards"`
}

func NewOpsHandler(userSvc *service.UserService, achievementSvc *service.AchievementService, restrictionSvc *service.AccountRestrictionService, internalToken string) *OpsHandler {
	return &OpsHandler{
		userSvc:        userSvc,
		achievementSvc: achievementSvc,
		restrictionSvc: restrictionSvc,
		internalToken:  strings.TrimSpace(internalToken),
	}
}

func (h *OpsHandler) RefreshAchievementByPhone(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	if h == nil || h.userSvc == nil || h.achievementSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "ops service not configured"})
		return
	}
	var body struct {
		Phone string `json:"phone"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	phone := strings.TrimSpace(body.Phone)
	if phone == "" {
		c.JSON(http.StatusBadRequest, utils.H{"error": "phone is required"})
		return
	}
	user, err := h.userSvc.FindByPhone(ctx, phone)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, utils.H{"error": "user not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	newRewards, err := h.achievementSvc.SettleUserCompletedTracksWithRewards(ctx, user.ID)
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	rewardList, err := h.achievementSvc.ListRewards(ctx, user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	earnedCount := 0
	if rewardList != nil {
		for _, reward := range rewardList.Rewards {
			if reward != nil && reward.Earned {
				earnedCount++
			}
		}
	}
	result := OpsAchievementRefreshResult{
		UserID:            user.ID,
		Phone:             user.Phone,
		NewRewardCount:    len(newRewards),
		EarnedRewardCount: earnedCount,
		NewRewards:        newRewards,
	}
	if rewardList != nil {
		result.QualifiedTrackCount = int64(rewardList.Stats.QualifiedTrackCount)
		result.TotalXP = rewardList.Stats.TotalXP
		result.CurrentLevel = rewardList.Stats.CurrentLevel
		result.NextLevel = rewardList.Stats.NextLevel
	}
	c.JSON(http.StatusOK, successResponse(result))
}

func (h *OpsHandler) CreateAccountRestriction(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	if h == nil || h.restrictionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "account restriction service not configured"})
		return
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid user_id"})
		return
	}
	var body struct {
		Reason    string `json:"reason"`
		Operator  string `json:"operator"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(c.Request.Body(), &body); err != nil {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid payload"})
		return
	}
	var expiresAt *time.Time
	if strings.TrimSpace(body.ExpiresAt) != "" {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(body.ExpiresAt))
		if err != nil {
			c.JSON(http.StatusBadRequest, utils.H{"error": "expires_at must be RFC3339"})
			return
		}
		expiresAt = &parsed
	}
	item, err := h.restrictionSvc.Create(ctx, service.CreateAccountRestrictionInput{
		UserID:    userID,
		Reason:    body.Reason,
		Operator:  body.Operator,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		var iae *service.InvalidArgumentError
		if errors.As(err, &iae) {
			c.JSON(http.StatusBadRequest, utils.H{"error": iae.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(item))
}

func (h *OpsHandler) GetCurrentAccountRestriction(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	if h == nil || h.restrictionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "account restriction service not configured"})
		return
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid user_id"})
		return
	}
	item, err := h.restrictionSvc.FindActive(ctx, userID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			c.JSON(http.StatusNotFound, utils.H{"error": "account restriction not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(item))
}

func (h *OpsHandler) ListAccountRestrictions(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	if h == nil || h.restrictionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "account restriction service not configured"})
		return
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid user_id"})
		return
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(string(c.Query("limit"))))
	items, err := h.restrictionSvc.ListByUserID(ctx, userID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(utils.H{"items": items}))
}

func (h *OpsHandler) RevokeCurrentAccountRestriction(ctx context.Context, c *app.RequestContext) {
	if !h.checkInternalToken(c) {
		return
	}
	if h == nil || h.restrictionSvc == nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": "account restriction service not configured"})
		return
	}
	userID, err := strconv.ParseInt(strings.TrimSpace(c.Param("user_id")), 10, 64)
	if err != nil || userID <= 0 {
		c.JSON(http.StatusBadRequest, utils.H{"error": "invalid user_id"})
		return
	}
	operator := strings.TrimSpace(string(c.Query("operator")))
	count, err := h.restrictionSvc.RevokeActive(ctx, userID, operator)
	if err != nil {
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, successResponse(utils.H{"revoked_count": count}))
}

func (h *OpsHandler) checkInternalToken(c *app.RequestContext) bool {
	if h == nil || h.internalToken == "" {
		c.JSON(http.StatusServiceUnavailable, utils.H{"error": "ops internal auth not configured"})
		return false
	}
	token := strings.TrimSpace(string(c.Request.Header.Peek("X-Internal-Token")))
	if token == "" {
		c.JSON(http.StatusUnauthorized, utils.H{"error": "missing internal token"})
		return false
	}
	if token != h.internalToken {
		c.JSON(http.StatusForbidden, utils.H{"error": "forbidden"})
		return false
	}
	return true
}
