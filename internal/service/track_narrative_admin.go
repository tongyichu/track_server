package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

// PreviewShape generates a reviewable suggestion without changing the track.
func (s *TrackNarrativeService) PreviewShape(ctx context.Context, id string) (*TrackNarrativeResult, error) {
	if s == nil {
		return nil, errors.New("AI 标签服务未配置")
	}
	if !s.mu.TryLock() {
		return nil, errors.New("AI 任务正在运行，请稍后重试")
	}
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
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
	day := now.In(time.FixedZone("Asia/Shanghai", 8*3600)).Format("2006-01-02")
	reserved, err := s.store.ReservePlaceAPICall(ctx, narrativeBudgetProvider, day, s.budget)
	if err != nil {
		return nil, err
	}
	if !reserved {
		return nil, errors.New("今日 AI 调用额度已用完，请明天重试")
	}
	raw, err := json.Marshal(snap.Input)
	if err != nil {
		return nil, err
	}
	result, err := s.generator.Generate(ctx, string(raw), image)
	if err != nil {
		return nil, errors.New("AI 识别失败，请稍后重试")
	}
	current, err := s.snapshot(ctx, id)
	if err != nil || current.Version != snap.Version {
		return nil, errors.New("轨迹资料已变化，请重新打开后生成")
	}
	return result, nil
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
	return map[string]interface{}{"status": status, "reason": reason, "attempts": state.Attempts, "first_seen": state.FirstSeen, "next_attempt": state.NextAttempt}, nil
}
