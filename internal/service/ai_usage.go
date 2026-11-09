package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/ai"
	"github.com/tongyichu/track_server/internal/models"
)

func (s *TrackNarrativeService) SetAIManager(m *ai.Manager) { s.ai = m }
func narrativeAISummary(s *trackNarrativeSnapshot) string {
	return fmt.Sprintf("%s；运动：%s；位置：%s %s；主景区：%s；标签：%d 项；使用已保存统计与一张轨迹方图，未发送原始 GPS", s.Input.Title, s.Input.Sport, s.Input.City, s.Input.Location, s.Input.ScenicName, len(s.Input.Labels))
}
func narrativeTriggerFields(s *trackNarrativeSnapshot) map[string]string {
	out := map[string]string{}
	for k, v := range map[string]any{"标题": s.Input.Title, "位置": []string{s.Input.City, s.Input.Location}, "运动数据": []any{s.Input.Sport, s.Input.DistanceKM, s.Input.DurationMinutes, s.Input.Gain, s.Input.ElevationSamples}, "标签": s.Input.Labels, "景区资料": []string{s.Input.ScenicName, s.Input.ScenicDescription, s.Input.ScenicCategory}, "投稿": s.Input.Submission, "轨迹预览": s.PreviewURL} {
		b, _ := json.Marshal(v)
		out[k] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	return out
}
func narrativeTriggerReason(old narrativeState, s *trackNarrativeSnapshot) string {
	if old.Version == "" {
		return "新增或上传后的首次生成"
	}
	if old.FactsVersion == s.FactsVersion {
		return "规则或模型版本变化"
	}
	now := narrativeTriggerFields(s)
	var changes []string
	for _, k := range []string{"标题", "位置", "运动数据", "标签", "景区资料", "投稿", "轨迹预览"} {
		if old.TriggerFields[k] != "" && old.TriggerFields[k] != now[k] {
			changes = append(changes, k)
		}
	}
	if len(changes) == 0 {
		return "已保存的轨迹资料或标签变化"
	}
	return strings.Join(changes, "、") + "变化"
}

// Retry reads current authoritative inputs and shares the existing scan lease; it never replays saved prompts.
func (s *TrackNarrativeService) RetryAI(ctx context.Context, t *models.AITask) error {
	switch t.Feature {
	case "track_description":
		_, err := s.PreviewDescription(ctx, t.ObjectID)
		return err
	case "track_shape":
		_, err := s.PreviewShape(ctx, t.ObjectID)
		return err
	}
	if !s.mu.TryLock() {
		return ai.ErrBusy
	}
	defer s.mu.Unlock()
	owner, err := submissionID("NAR-RETRY-")
	if err != nil {
		return err
	}
	now := time.Now()
	ok, err := s.store.LeasePlaceRecord(ctx, narrativeScanKey, owner, now, now.Add(7*time.Minute))
	if err != nil {
		return err
	}
	if !ok {
		return ai.ErrBusy
	}
	var scan narrativeScan
	if err = s.readState(ctx, narrativeScanKey, &scan); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.saveState(cleanup, narrativeScanKey, scan, owner)
	}()
	switch t.Feature {
	case "track_auto":
		snap, err := s.snapshot(ctx, t.ObjectID)
		if err != nil {
			return err
		}
		var state narrativeState
		if err = s.readState(ctx, "narrative:track:"+t.ObjectID, &state); err != nil {
			return err
		}
		state.Version = snap.Version
		state.FactsVersion = snap.FactsVersion
		state.Status = "waiting"
		state.Attempts = 0
		state.NextAttempt = time.Time{}
		state.FirstSeen = time.Now().Add(-11 * time.Minute)
		state.Result = nil
		state.TriggerReason = "后台手动重试"
		if err = s.saveState(ctx, "narrative:track:"+t.ObjectID, state, ""); err != nil {
			return err
		}
		_, err = s.process(ctx, t.ObjectID, time.Time{}, &TrackNarrativeRunResult{})
		return err
	case "shape_learning":
		err = s.editShapeFeedback(ctx, t.ObjectID, func(f *shapeFeedback) error {
			if f.Revision == "" {
				return errors.New("当前没有可复核的人工修正")
			}
			f.Status = "pending"
			f.Attempts = 0
			f.NextAttempt = time.Time{}
			f.Review = nil
			return nil
		})
		if err != nil {
			return err
		}
		_, err = s.learnShapeCorrection(ctx, t.ObjectID)
		return err
	}
	return errors.New("不支持的 AI 重试功能")
}

func (s *TrackNarrativeService) ValidateShapeTask(ctx context.Context, id, taskID string) error {
	if taskID == "" {
		return nil
	}
	snap, err := s.snapshot(ctx, id)
	if err != nil {
		return err
	}
	return s.ai.ValidateCandidate(ctx, taskID, "track_shape", id, snap.Version)
}
func (s *TrackNarrativeService) MarkShapeAdopted(ctx context.Context, id, taskID string) error {
	return s.ai.MarkAdopted(ctx, taskID, "track_shape", id)
}
