package service

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

const (
	achievementRecentLimit = 10
	metersPerKM            = 1000
)

// AchievementService manages achievement definitions, stats and reward settlement.
type AchievementService struct {
	rewards repository.AchievementRepository
	tracks  repository.TrackRepository
}

func NewAchievementService(rewards repository.AchievementRepository, tracks repository.TrackRepository) *AchievementService {
	return &AchievementService{rewards: rewards, tracks: tracks}
}

func (s *AchievementService) Definitions() []models.AchievementDefinition {
	return append([]models.AchievementDefinition(nil), achievementDefinitions...)
}

func (s *AchievementService) GetSummary(ctx context.Context, userID int64) (*models.AchievementSummary, error) {
	stats, earned, err := s.buildUserState(ctx, userID)
	if err != nil {
		return nil, err
	}
	recent, err := s.recentRewardViews(ctx, userID, earned, stats, achievementRecentLimit)
	if err != nil {
		return nil, err
	}
	return &models.AchievementSummary{Stats: *stats, RecentRewards: recent}, nil
}

func (s *AchievementService) ListRewards(ctx context.Context, userID int64) (*models.AchievementRewardList, error) {
	stats, earned, err := s.buildUserState(ctx, userID)
	if err != nil {
		return nil, err
	}
	views := make([]*models.AchievementRewardView, 0, len(achievementDefinitions))
	for _, def := range achievementDefinitions {
		views = append(views, s.rewardView(def, earned[def.Code], stats))
	}
	return &models.AchievementRewardList{Stats: *stats, Rewards: views}, nil
}

func (s *AchievementService) SettleTrackCompleted(ctx context.Context, track *models.Track) ([]*models.AchievementRewardView, error) {
	if s == nil || s.rewards == nil || track == nil || !isQualifiedAchievementTrack(track) {
		return nil, nil
	}
	stats, earned, err := s.buildUserState(ctx, track.UserID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	newRewards := make([]*models.AchievementRewardView, 0)
	for _, def := range achievementDefinitions {
		if earned[def.Code] != nil {
			continue
		}
		if achievementProgress(def, stats) < def.TargetValue {
			continue
		}
		reward := &models.UserAchievementReward{
			UserID:          track.UserID,
			RewardCode:      def.Code,
			SourceTrackID:   track.ID,
			SourceSessionID: track.SessionID,
			EarnedAt:        now,
		}
		created, err := s.rewards.UpsertUserReward(ctx, reward)
		if err != nil {
			return nil, err
		}
		if created {
			earned[def.Code] = reward
			newRewards = append(newRewards, s.rewardView(def, reward, stats))
		}
	}
	return newRewards, nil
}

func (s *AchievementService) buildUserState(ctx context.Context, userID int64) (*models.UserAchievementStats, map[string]*models.UserAchievementReward, error) {
	if userID <= 0 {
		return nil, nil, invalidArg("userID is required")
	}
	stats, err := s.aggregateStats(ctx, userID)
	if err != nil {
		return nil, nil, err
	}
	earned := make(map[string]*models.UserAchievementReward)
	if s.rewards != nil {
		rewards, err := s.rewards.ListUserRewards(ctx, userID)
		if err != nil {
			return nil, nil, err
		}
		for _, reward := range rewards {
			if reward != nil {
				earned[reward.RewardCode] = reward
			}
		}
	}
	return stats, earned, nil
}

func (s *AchievementService) aggregateStats(ctx context.Context, userID int64) (*models.UserAchievementStats, error) {
	stats := &models.UserAchievementStats{
		TypeStats: make(map[string]models.TrackTypeAchievementStats),
	}
	if s == nil || s.tracks == nil {
		stats.CurrentLevel = achievementLevels[0]
		return stats, nil
	}
	var cursor *models.TrackListCursor
	for {
		items, err := s.tracks.ListByUserID(ctx, userID, cursor, maxTrackPageSize)
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			break
		}
		for _, track := range items {
			if !isQualifiedAchievementTrack(track) {
				continue
			}
			stats.QualifiedTrackCount++
			stats.TotalDistance += track.Distance
			stats.TotalDuration += int64(track.Duration)
			stats.TotalElevationGain += int64(track.ElevationGain)
			if track.SessionID != "" {
				stats.CompanionCount++
			}
			typeStat := stats.TypeStats[normalizeAchievementTrackType(track.TrackType)]
			typeStat.Distance += track.Distance
			typeStat.Duration += int64(track.Duration)
			typeStat.ElevationGain += int64(track.ElevationGain)
			typeStat.TrackCount++
			if track.Distance > typeStat.MaxDistance {
				typeStat.MaxDistance = track.Distance
			}
			if int64(track.ElevationGain) > typeStat.MaxElevationGain {
				typeStat.MaxElevationGain = int64(track.ElevationGain)
			}
			typeStat.XP += achievementTrackXP(track)
			stats.TypeStats[normalizeAchievementTrackType(track.TrackType)] = typeStat
			stats.TotalXP += achievementTrackXP(track)
		}
		if len(items) < maxTrackPageSize {
			break
		}
		last := items[len(items)-1]
		cursor = &models.TrackListCursor{StartTime: last.StartTime, ID: last.ID}
	}
	stats.CurrentLevel, stats.NextLevel, stats.LevelProgress = resolveAchievementLevel(stats.TotalXP)
	return stats, nil
}

func (s *AchievementService) recentRewardViews(ctx context.Context, userID int64, earned map[string]*models.UserAchievementReward, stats *models.UserAchievementStats, limit int) ([]*models.AchievementRewardView, error) {
	if s == nil || s.rewards == nil {
		return nil, nil
	}
	rewards, err := s.rewards.ListRecentUserRewards(ctx, userID, limit)
	if err != nil {
		return nil, err
	}
	views := make([]*models.AchievementRewardView, 0, len(rewards))
	for _, reward := range rewards {
		def, ok := achievementDefinitionByCode(reward.RewardCode)
		if !ok {
			continue
		}
		if earned[reward.RewardCode] == nil {
			earned[reward.RewardCode] = reward
		}
		views = append(views, s.rewardView(def, reward, stats))
	}
	return views, nil
}

func (s *AchievementService) rewardView(def models.AchievementDefinition, reward *models.UserAchievementReward, stats *models.UserAchievementStats) *models.AchievementRewardView {
	current := achievementProgress(def, stats)
	progress := 0.0
	if def.TargetValue > 0 {
		progress = math.Min(1, current/def.TargetValue)
	}
	view := &models.AchievementRewardView{
		AchievementDefinition: def,
		Earned:                reward != nil,
		CurrentValue:          current,
		Progress:              progress,
	}
	if reward != nil {
		t := reward.EarnedAt
		view.EarnedAt = &t
		view.CurrentValue = def.TargetValue
		view.Progress = 1
	}
	return view
}

func isQualifiedAchievementTrack(track *models.Track) bool {
	if track == nil || track.IsRunning || track.Status != models.TrackStatusNormal {
		return false
	}
	trackType := normalizeAchievementTrackType(track.TrackType)
	minDistance := map[string]float64{"跑步": 500, "徒步": 500, "爬山": 300, "骑行": 1000, "自驾": 3000}
	minDuration := map[string]uint32{"跑步": 180, "徒步": 300, "爬山": 300, "骑行": 300, "自驾": 300}
	if minDistance[trackType] == 0 {
		return false
	}
	return track.Distance >= minDistance[trackType] && track.Duration >= minDuration[trackType]
}

func achievementTrackXP(track *models.Track) int64 {
	trackType := normalizeAchievementTrackType(track.TrackType)
	distanceKM := track.Distance / metersPerKM
	weights := map[string]float64{"跑步": 15, "徒步": 18, "爬山": 12, "骑行": 6, "自驾": 1}
	caps := map[string]float64{"跑步": 800, "徒步": 900, "爬山": 700, "骑行": 1200, "自驾": 600}
	distanceXP := distanceKM * weights[trackType]
	if capValue := caps[trackType]; capValue > 0 && distanceXP > capValue {
		distanceXP = capValue
	}
	durationXP := math.Min(float64(track.Duration)/600*5, 120)
	elevationXP := 0.0
	switch trackType {
	case "爬山":
		elevationXP = float64(track.ElevationGain) / 100 * 12
	case "骑行":
		elevationXP = float64(track.ElevationGain) / 100 * 6
	case "徒步":
		elevationXP = float64(track.ElevationGain) / 100 * 3
	}
	qualityXP := 0.0
	if track.RawTrackURL != "" {
		qualityXP += 20
	}
	if track.TrackScreenshotURL != "" || track.TrackNoMapBgScreenshotURL != "" {
		qualityXP += 10
	}
	if track.SessionID != "" {
		qualityXP += 30
	}
	total := distanceXP + durationXP + elevationXP + qualityXP
	if total < 0 {
		return 0
	}
	return int64(math.Round(total))
}

func normalizeAchievementTrackType(trackType string) string {
	t := strings.TrimSpace(trackType)
	switch t {
	case "跑步", "徒步", "爬山", "骑行", "自驾":
		return t
	case "骑车":
		return "骑行"
	default:
		return t
	}
}

func resolveAchievementLevel(xp int64) (models.AchievementLevel, *models.AchievementLevel, float64) {
	current := achievementLevels[0]
	var next *models.AchievementLevel
	for i := range achievementLevels {
		if xp >= achievementLevels[i].XP {
			current = achievementLevels[i]
			continue
		}
		n := achievementLevels[i]
		next = &n
		break
	}
	if next == nil {
		return current, nil, 1
	}
	span := next.XP - current.XP
	if span <= 0 {
		return current, next, 1
	}
	return current, next, float64(xp-current.XP) / float64(span)
}

func achievementProgress(def models.AchievementDefinition, stats *models.UserAchievementStats) float64 {
	if stats == nil {
		return 0
	}
	switch def.Code {
	case "first_track":
		return float64(stats.QualifiedTrackCount)
	case "track_count_10":
		return float64(stats.QualifiedTrackCount)
	case "track_count_100":
		return float64(stats.QualifiedTrackCount)
	case "first_companion":
		return float64(stats.CompanionCount)
	case "run_5k", "run_10k":
		return typeMaxDistanceKM(stats, "跑步")
	case "hike_3k", "hike_15k":
		return typeMaxDistanceKM(stats, "徒步")
	case "climb_100m", "climb_1000m":
		return float64(stats.TypeStats["爬山"].MaxElevationGain)
	case "ride_10k", "ride_50k":
		return typeMaxDistanceKM(stats, "骑行")
	case "drive_30k", "drive_300k":
		return typeMaxDistanceKM(stats, "自驾")
	}
	switch def.Category {
	case "跑步":
		return typeDistanceKM(stats, "跑步")
	case "徒步":
		return typeDistanceKM(stats, "徒步")
	case "爬山":
		return float64(stats.TypeStats["爬山"].ElevationGain)
	case "骑行":
		return typeDistanceKM(stats, "骑行")
	case "自驾":
		return typeDistanceKM(stats, "自驾")
	}
	return 0
}

func typeDistanceKM(stats *models.UserAchievementStats, trackType string) float64 {
	if stats == nil {
		return 0
	}
	return stats.TypeStats[trackType].Distance / metersPerKM
}

func typeMaxDistanceKM(stats *models.UserAchievementStats, trackType string) float64 {
	if stats == nil {
		return 0
	}
	return stats.TypeStats[trackType].MaxDistance / metersPerKM
}

func achievementDefinitionByCode(code string) (models.AchievementDefinition, bool) {
	for _, def := range achievementDefinitions {
		if def.Code == code {
			return def, true
		}
	}
	return models.AchievementDefinition{}, false
}

var achievementLevels = []models.AchievementLevel{
	{Level: 1, Name: "初上路", XP: 0},
	{Level: 2, Name: "熟悉路线", XP: 300},
	{Level: 3, Name: "周末行者", XP: 1000},
	{Level: 4, Name: "路线探索者", XP: 3000},
	{Level: 5, Name: "长线玩家", XP: 8000},
	{Level: 6, Name: "山野熟客", XP: 18000},
	{Level: 7, Name: "轨迹达人", XP: 40000},
	{Level: 8, Name: "路线大师", XP: 80000},
	{Level: 9, Name: "远征者", XP: 150000},
	{Level: 10, Name: "传奇轨迹家", XP: 300000},
}

var achievementDefinitions = []models.AchievementDefinition{
	{Code: "first_track", Type: models.AchievementRewardTypeMilestone, Category: "通用", Name: "第一条轨迹", Description: "完成首条有效轨迹", Rarity: models.AchievementRarityCommon, TargetValue: 1},
	{Code: "track_count_10", Type: models.AchievementRewardTypeMilestone, Category: "通用", Name: "第 10 条轨迹", Description: "累计完成 10 条有效轨迹", Rarity: models.AchievementRarityRare, TargetValue: 10},
	{Code: "track_count_100", Type: models.AchievementRewardTypeMilestone, Category: "通用", Name: "第 100 条轨迹", Description: "累计完成 100 条有效轨迹", Rarity: models.AchievementRarityEpic, TargetValue: 100},
	{Code: "first_companion", Type: models.AchievementRewardTypeBadge, Category: "同行", Name: "首次同行", Description: "完成首条关联同行的有效轨迹", Rarity: models.AchievementRarityCommon, TargetValue: 1},

	{Code: "run_5k", Type: models.AchievementRewardTypeBadge, Category: "跑步", Name: "5K 完成", Description: "单次跑步距离达到 5km", Rarity: models.AchievementRarityCommon, TargetValue: 5},
	{Code: "run_10k", Type: models.AchievementRewardTypeBadge, Category: "跑步", Name: "10K 完成", Description: "单次跑步距离达到 10km", Rarity: models.AchievementRarityRare, TargetValue: 10},
	{Code: "run_100k", Type: models.AchievementRewardTypeBadge, Category: "跑步", Name: "跑步百公里", Description: "累计跑步距离达到 100km", Rarity: models.AchievementRarityRare, TargetValue: 100},
	{Code: "run_1000k", Type: models.AchievementRewardTypeBadge, Category: "跑步", Name: "跑步千公里", Description: "累计跑步距离达到 1000km", Rarity: models.AchievementRarityEpic, TargetValue: 1000},

	{Code: "hike_3k", Type: models.AchievementRewardTypeBadge, Category: "徒步", Name: "徒步初体验", Description: "单次徒步距离达到 3km", Rarity: models.AchievementRarityCommon, TargetValue: 3},
	{Code: "hike_15k", Type: models.AchievementRewardTypeBadge, Category: "徒步", Name: "长线徒步", Description: "单次徒步距离达到 15km", Rarity: models.AchievementRarityRare, TargetValue: 15},
	{Code: "hike_100k", Type: models.AchievementRewardTypeBadge, Category: "徒步", Name: "徒步百公里", Description: "累计徒步距离达到 100km", Rarity: models.AchievementRarityRare, TargetValue: 100},
	{Code: "hike_1000k", Type: models.AchievementRewardTypeBadge, Category: "徒步", Name: "徒步千公里", Description: "累计徒步距离达到 1000km", Rarity: models.AchievementRarityEpic, TargetValue: 1000},
	{Code: "hike_3000k", Type: models.AchievementRewardTypeBadge, Category: "徒步", Name: "徒步三千里", Description: "累计徒步距离达到 3000km", Rarity: models.AchievementRarityLegendary, TargetValue: 3000},

	{Code: "climb_100m", Type: models.AchievementRewardTypeBadge, Category: "爬山", Name: "小坡热身", Description: "单次爬升达到 100m", Rarity: models.AchievementRarityCommon, TargetValue: 100},
	{Code: "climb_1000m", Type: models.AchievementRewardTypeBadge, Category: "爬山", Name: "登高挑战", Description: "单次爬升达到 1000m", Rarity: models.AchievementRarityEpic, TargetValue: 1000},
	{Code: "climb_8848m", Type: models.AchievementRewardTypeBadge, Category: "爬山", Name: "珠峰累计", Description: "累计爬升达到 8848m", Rarity: models.AchievementRarityEpic, TargetValue: 8848},
	{Code: "climb_100000m", Type: models.AchievementRewardTypeBadge, Category: "爬山", Name: "十万米爬升", Description: "累计爬升达到 100000m", Rarity: models.AchievementRarityLegendary, TargetValue: 100000},

	{Code: "ride_10k", Type: models.AchievementRewardTypeBadge, Category: "骑行", Name: "骑行起步", Description: "单次骑行距离达到 10km", Rarity: models.AchievementRarityCommon, TargetValue: 10},
	{Code: "ride_50k", Type: models.AchievementRewardTypeBadge, Category: "骑行", Name: "半百骑行", Description: "单次骑行距离达到 50km", Rarity: models.AchievementRarityRare, TargetValue: 50},
	{Code: "ride_1000k", Type: models.AchievementRewardTypeBadge, Category: "骑行", Name: "骑行千公里", Description: "累计骑行距离达到 1000km", Rarity: models.AchievementRarityEpic, TargetValue: 1000},
	{Code: "ride_5000k", Type: models.AchievementRewardTypeBadge, Category: "骑行", Name: "骑行五千公里", Description: "累计骑行距离达到 5000km", Rarity: models.AchievementRarityLegendary, TargetValue: 5000},

	{Code: "drive_30k", Type: models.AchievementRewardTypeBadge, Category: "自驾", Name: "城郊兜风", Description: "单次自驾距离达到 30km", Rarity: models.AchievementRarityCommon, TargetValue: 30},
	{Code: "drive_300k", Type: models.AchievementRewardTypeBadge, Category: "自驾", Name: "长途驾驶", Description: "单次自驾距离达到 300km", Rarity: models.AchievementRarityRare, TargetValue: 300},
	{Code: "drive_10000k", Type: models.AchievementRewardTypeBadge, Category: "自驾", Name: "公路万里", Description: "累计自驾距离达到 10000km", Rarity: models.AchievementRarityLegendary, TargetValue: 10000},
	{Code: "drive_30000k", Type: models.AchievementRewardTypeBadge, Category: "自驾", Name: "山河三万里", Description: "累计自驾距离达到 30000km", Rarity: models.AchievementRarityLegendary, TargetValue: 30000},
}
