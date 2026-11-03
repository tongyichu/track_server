package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

// PreviewShape generates a reviewable suggestion without changing the track.
func (s *TrackNarrativeService) PreviewShape(ctx context.Context, id string) (*TrackShapeResult, error) {
	if s == nil {
		return nil, errors.New("AI 标签服务未配置")
	}
	generator, ok := s.generator.(TrackShapeGenerator)
	if !ok {
		return nil, errors.New("专用形象识别服务未配置")
	}
	if !s.mu.TryLock() {
		return nil, errors.New("AI 任务正在运行，请稍后重试")
	}
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 270*time.Second)
	defer cancel()
	owner, err := submissionID("NAR-ADMIN-")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	acquired, err := s.store.LeasePlaceRecord(ctx, narrativeScanKey, owner, now, now.Add(5*time.Minute))
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, errors.New("AI 任务正在运行，请稍后重试")
	}
	var scan narrativeScan
	if err = s.readState(ctx, narrativeScanKey, &scan); err != nil {
		return nil, err
	}
	if scan.ActivatedAt.IsZero() {
		scan.ActivatedAt = s.startedAt
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.saveState(releaseCtx, narrativeScanKey, scan, owner)
	}()
	snap, err := s.snapshot(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, errors.New("需公开且已结束的轨迹，并准备好地理索引与方形预览图")
	}
	if err != nil {
		return nil, err
	}
	image, err := s.trackSvc.TrackPreviewFile(ctx, snap.Track.UserID, id, snap.PreviewVariant, previewVersion(snap.PreviewURL))
	if err != nil {
		return nil, errors.New("预览图读取失败，请稍后重试")
	}
	snap.Input.LearnedShapeExamples, err = s.learnedShapeExamples(ctx)
	if err != nil {
		return nil, err
	}
	day := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	reserved, err := s.store.ReservePlaceAPICall(ctx, narrativeBudgetProvider, day, s.budget)
	if err != nil {
		return nil, err
	}
	if !reserved {
		return nil, errors.New("今日 AI 调用额度已用完，请明天重试")
	}
	// Shape-only input deliberately excludes narrative, submission and scenic prose.
	raw, err := json.Marshal(map[string]interface{}{"title": snap.Input.Title, "sport": snap.Input.Sport, "distance_km": snap.Input.DistanceKM, "recorded_duration_minutes": snap.Input.DurationMinutes, "recorded_gain_m": snap.Input.Gain, "learned_shape_examples": snap.Input.LearnedShapeExamples})
	if err != nil {
		return nil, err
	}
	result, err := generator.GenerateShape(ctx, string(raw), image)
	if err != nil {
		log.Printf("[TrackShape] track_id=%s generation_failed: %v", id, err)
		if errors.Is(err, errKimiOutputLimit) {
			return nil, errors.New("AI 输出达到单次上限，未获得完整结果，请重试")
		}
		return nil, errors.New("AI 识别失败，请稍后重试")
	}
	current, err := s.snapshot(ctx, id)
	if err != nil || current.Version != snap.Version {
		return nil, errors.New("轨迹资料已变化，请重新打开后生成")
	}
	if err = s.saveState(ctx, "narrative:shape-preview:"+id, trackShapePreview{Version: snap.Version, Model: s.model, CreatedAt: time.Now(), Result: result}, ""); err != nil {
		log.Printf("[TrackShape] track_id=%s preview_save_failed: %v", id, err)
		return nil, errors.New("AI 已返回结果，但识别依据保存失败，请重试")
	}
	return result, nil
}

type trackShapePreview struct {
	Version   string            `json:"version"`
	Model     string            `json:"model"`
	CreatedAt time.Time         `json:"created_at"`
	Result    *TrackShapeResult `json:"result"`
}

func (s *TrackNarrativeService) lastShapePreview(ctx context.Context, id string) *trackShapePreview {
	var preview trackShapePreview
	if s.readState(ctx, "narrative:shape-preview:"+id, &preview) != nil || preview.Result == nil {
		return nil
	}
	current, err := s.snapshot(ctx, id)
	if err != nil || current.Version != preview.Version {
		return nil
	}
	return &preview
}

func (s *TrackNarrativeService) AdminStatus(ctx context.Context, id string) (interface{}, error) {
	if s == nil {
		return map[string]string{"status": "unconfigured"}, nil
	}
	var state narrativeState
	if err := s.readState(ctx, "narrative:track:"+id, &state); err != nil {
		return nil, err
	}
	status := state.Status
	if status == "" {
		status = "not_queued"
	}
	reason := ""
	if status == "waiting" || status == "retry" {
		if state.NextAttempt.After(time.Now()) {
			reason = "等待防抖或重试时间"
		}
		if snap, err := s.snapshot(ctx, id); err == nil && !snap.PlaceReady && time.Since(state.FirstSeen) < 10*time.Minute {
			reason = "等待景点识别资料（入队后最多等待十分钟）"
		}
		if reason == "" {
			reason = "等待后台轮转或可用调用额度"
		}
	}
	return map[string]interface{}{"status": status, "reason": reason, "attempts": state.Attempts, "first_seen": state.FirstSeen, "next_attempt": state.NextAttempt, "learning": s.shapeLearningStatus(ctx, id), "shape_preview": s.lastShapePreview(ctx, id)}, nil
}
