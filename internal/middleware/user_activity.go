package middleware

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/tongyichu/track_server/internal/service"
)

// UserActivityMiddleware must run after business JWT authentication, outside admin/internal routes.
func UserActivityMiddleware(users *service.UserService) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		meta := GetRequestMeta(c)
		var userID int64
		if meta != nil {
			userID = meta.AuthUserID
		}
		c.Next(ctx)
		if userID <= 0 || strings.HasPrefix(string(c.Path()), "/api/v1/static/") || c.Response.StatusCode() >= http.StatusInternalServerError || c.Response.StatusCode() == http.StatusUnauthorized {
			return
		}
		users.RecordActivity(ctx, userID)
	}
}
