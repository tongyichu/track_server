package service

import (
	"context"
	"strings"
	"time"

	"github.com/tongyichu/track_server/internal/models"
	"github.com/tongyichu/track_server/internal/repository"
)

func (s *FeedbackService) updateCorrectionStatus(ctx context.Context, item *models.Feedback, in UpdateFeedbackStatusInput, reply string) error {
	c := *item.Correction
	if in.Revision != nil && *in.Revision != c.Revision {
		return repository.ErrAlreadyExists
	}
	outcome := strings.TrimSpace(in.Outcome)
	terminal := in.Status == models.FeedbackStatusResolved || in.Status == models.FeedbackStatusIgnored
	if !terminal && outcome != "" {
		return invalidArg("未完成的纠错不能设置处理结果")
	}
	switch outcome {
	case "":
		if terminal && in.Revision != nil {
			return invalidArg("请选择纠错处理结果")
		}
	case "corrected", "unchanged":
		if in.Status != models.FeedbackStatusResolved {
			return invalidArg("已修正和无需修改应使用已处理状态")
		}
	case "unverified":
		if in.Status != models.FeedbackStatusIgnored {
			return invalidArg("暂无法核实应使用已关闭状态")
		}
	default:
		return invalidArg("无效的纠错处理结果")
	}
	if terminal && outcome != "" && reply == "" {
		return ErrFeedbackReplyRequired
	}
	// Older callers may update a reply without sending the new result fields.
	if in.Revision == nil && outcome == "" && item.Status == in.Status {
		outcome = c.Outcome
	}
	c.Outcome = outcome
	preserveHandler := in.Operator == "" && in.Revision == nil && in.Outcome == "" && item.Status == in.Status
	if !terminal {
		c.HandledAt, c.HandledBy = nil, ""
	} else if !preserveHandler {
		now := time.Now().UTC()
		c.HandledAt, c.HandledBy = &now, in.Operator
	}
	c.Revision++
	return s.repo.UpdateCorrectionStatus(ctx, item.FeedbackID, in.Status, reply, &c, item.Correction.Revision)
}

// RecordCorrectionTrackChange is called only after the associated track was saved.
func (s *FeedbackService) RecordCorrectionTrackChange(ctx context.Context, id, trackID, operator string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		item, err := s.GetOps(ctx, id)
		if err != nil {
			return err
		}
		if item.Correction == nil || item.Correction.TrackID != trackID {
			return invalidArg("纠错记录与轨迹不匹配")
		}
		c := *item.Correction
		c.TrackChange = &models.FeedbackTrackChange{Operator: operator, SavedAt: time.Now().UTC(), Fields: append([]string(nil), fields...)}
		c.Revision++
		err = s.repo.UpdateCorrectionStatus(ctx, id, item.Status, item.Reply, &c, item.Correction.Revision)
		if err != repository.ErrAlreadyExists {
			return err
		}
	}
	return repository.ErrAlreadyExists
}
