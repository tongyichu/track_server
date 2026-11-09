package service

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/repository"
)

// PreviewDescription bypasses the historical watermark but never writes track fields or queues a batch.
func (s *TrackNarrativeService) PreviewDescription(ctx context.Context, id string) (*TrackDescriptionResult, error) {
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
	// This operation changes only description, so it must not advertise an unsaved shape suggestion.
	snap.Input.ShapeLocked = true
	facts, err := json.Marshal(snap.Input)
	if err != nil {
		return nil, err
	}
	query := strings.TrimSpace(snap.Input.City+" "+place) + " 官方 地理 景点 户外介绍"
	result, err := generator.GenerateDescription(ctx, string(facts), image, query)
	if err != nil {
		log.Printf("[TrackDescription] track_id=%s generation_failed: %v", id, err)
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
	return result, nil
}
