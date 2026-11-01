package admin

import (
	"reflect"
	"sort"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/tongyichu/track_server/internal/models"
)

func (h *Handler) canHandleCorrection(c *app.RequestContext) bool {
	if h.auth == nil {
		return false
	}
	session := h.auth.SessionFromRequest(c)
	return session != nil && session.Account != nil && session.Account.allows("feedbacks", true)
}

func adminFeedbackDetail(item *models.Feedback) any {
	view := struct {
		*models.Feedback
		HandledBy   string                      `json:"handled_by,omitempty"`
		TrackChange *models.FeedbackTrackChange `json:"track_change,omitempty"`
	}{Feedback: item}
	if item.Correction != nil {
		view.HandledBy = item.Correction.HandledBy
		view.TrackChange = item.Correction.TrackChange
	}
	return view
}

func correctionChangedFields(before, after *models.Track) []string {
	a, b := reflect.ValueOf(*before), reflect.ValueOf(*after)
	fields := []string{}
	for i := 0; i < a.NumField(); i++ {
		name := a.Type().Field(i).Name
		if name == "UpdatedAt" || name == "CreatedAt" {
			continue
		}
		if !reflect.DeepEqual(a.Field(i).Interface(), b.Field(i).Interface()) {
			fields = append(fields, name)
		}
	}
	sort.Strings(fields)
	return fields
}
