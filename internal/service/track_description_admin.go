package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tongyichu/track_server/internal/ai"
	"github.com/tongyichu/track_server/internal/repository"
)

// PreviewDescription bypasses the historical watermark but never writes track fields or queues a batch.
func (s *TrackNarrativeService) PreviewDescription(ctx context.Context, id string) (result *TrackDescriptionResult, err error) {
	if s == nil {
		return nil, errors.New("AI 路线说明服务未配置")
	}
	generator, ok := s.generator.(TrackDescriptionGenerator)
	if !ok {
		return nil, errors.New("AI 联网填写服务未配置")
	}
	if !s.mu.TryLock() {
		return nil, errors.New("AI 任务正在运行，请稍后重试")
	}
	defer s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 270*time.Second)
	defer cancel()
	owner, err := submissionID("NAR-DESC-")
	if err != nil {
		return nil, err
	}
	now := time.Now()
	acquired, err := s.store.LeasePlaceRecord(ctx, narrativeScanKey, owner, now, now.Add(5*time.Minute))
	if err != nil || !acquired {
		return nil, errors.New("AI 任务正在运行，请稍后重试")
	}
	var scan narrativeScan
	if err := s.readState(ctx, narrativeScanKey, &scan); err != nil {
		return nil, err
	}
	defer func() {
		releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.saveState(releaseCtx, narrativeScanKey, scan, owner); err != nil {
			log.Printf("[TrackDescription] lease_release_failed: %v", err)
		}
	}()
	snap, err := s.snapshot(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, errors.New("需公开且已结束的轨迹，并准备好地理索引与方形预览图")
	}
	if err != nil {
		return nil, err
	}
	place := snap.Input.ScenicName
	if place == "" {
		place = snap.Input.Location
	}
	if strings.TrimSpace(place) == "" {
		return nil, errors.New("请先保存位置信息或关联景点，再联网填写")
	}
	ctx, run, err := s.ai.Begin(ctx, ai.Spec{Feature: "track_description", Provider: "moonshot", Model: s.model, ObjectID: id, ObjectName: snap.Track.Title, Mode: "manual", Reason: "后台 AI 联网填写", InputVersion: trackDescriptionVersion(snap), RuleVersion: fmt.Sprint(trackNarrativeRuleVersion), InputSummary: narrativeAISummary(snap)})
	if err != nil {
		return nil, err
	}
	defer func() { run.Finish(err) }()
	image, err := s.trackSvc.TrackPreviewFile(ctx, snap.Track.UserID, id, snap.PreviewVariant, previewVersion(snap.PreviewURL))
	if err != nil {
		return nil, errors.New("预览图读取失败，请稍后重试")
	}

	// This operation changes only description, so it must not advertise an unsaved shape suggestion.
	snap.Input.ShapeLocked = true
	facts, err := json.Marshal(snap.Input)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(snap.Input.City+" "+place) + " 官方 地理 景点 户外介绍"
	result, err = generator.GenerateDescription(ctx, string(facts), image, query)
	if err != nil {
		log.Printf("[TrackDescription] track_id=%s generation_failed: %v", id, err)
		if ai.IsControlError(err) {
			return nil, err
		}
		if e := run.Check(); e != nil {
			return nil, e
		}
		if errors.Is(err, errKimiInsufficientBalance) || errors.Is(err, errKimiSearchDenied) {
			return nil, err
		}
		if errors.Is(err, errKimiOutputLimit) {
			return nil, errors.New("AI 输出达到单次上限，未获得完整说明，请重试")
		}
		return nil, errors.New("AI 联网填写失败，未修改原说明，请稍后重试")
	}
	current, err := s.snapshot(ctx, id)
	if err != nil || current.Version != snap.Version || current.Track.ShapeLabel != snap.Track.ShapeLabel || current.Track.Description != snap.Track.Description || current.Track.DescriptionSource != snap.Track.DescriptionSource {
		return nil, errors.New("轨迹资料已变化，请重新打开后生成")
	}
	result.SourceVersion = trackDescriptionVersion(current)
	result.TaskID = ai.TaskID(ctx)
	if err = run.Result(result, "pending"); err != nil {
		return nil, err
	}
	return result, nil
}

func trackDescriptionVersion(snapshot *trackNarrativeSnapshot) string {
	t := snapshot.Track
	raw, _ := json.Marshal([]interface{}{snapshot.Version, t.ID, t.UpdatedAt.UTC(), t.Description, t.DescriptionSource, t.DescriptionRuleVersion, t.ShapeLabel})
	return fmt.Sprintf("%x", sha256.Sum256(raw))
}

func (s *TrackNarrativeService) ApplyDescription(ctx context.Context, id, version, description string, taskIDs ...string) (string, error) {
	if s == nil {
		return "", errors.New("路线说明保存服务未配置")
	}
	description = strings.TrimSpace(description)
	if description == "" || utf8.RuneCountInString(description) > 4000 || strings.TrimSpace(version) == "" {
		return "", errors.New("请先生成有效说明，说明最多4000个字符")
	}
	current, err := s.snapshot(ctx, id)
	if err != nil {
		return "", errors.New("轨迹资料不可用，请重新打开后生成")
	}
	if version != trackDescriptionVersion(current) {
		return "", errors.New("轨迹资料已变化，请重新生成说明后代入")
	}
	taskID := ""
	if len(taskIDs) > 0 {
		taskID = taskIDs[0]
	}
	if err := s.ai.ValidateCandidate(ctx, taskID, "track_description", id, version); err != nil {
		return "", err
	}
	writer, ok := s.tracks.(repository.TrackDescriptionWriter)
	if !ok {
		return "", errors.New("路线说明保存服务未配置")
	}
	applied, err := writer.ApplyTrackDescription(ctx, current.Track, description)
	if err != nil {
		log.Printf("[TrackDescription] track_id=%s save_failed: %v", id, err)
		return "", errors.New("路线说明保存失败，请稍后重试；建议内容已保留")
	}
	if !applied {
		return "", errors.New("轨迹资料已变化，请重新生成说明后代入")
	}
	if err := s.ai.MarkAdopted(ctx, taskID, "track_description", id); err != nil {
		log.Printf("AI description saved but adoption record unavailable: task=%s", taskID)
	}
	return description, nil
}
