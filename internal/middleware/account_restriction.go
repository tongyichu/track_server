package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/tongyichu/track_server/internal/repository"
	"github.com/tongyichu/track_server/internal/service"
)

type restrictedRoute struct {
	method  string
	path    string
	prefix  string
	suffix  string
	message string
}

var accountRestrictedRoutes = []restrictedRoute{
	{method: http.MethodGet, path: "/api/v1/oss/sts-token", message: service.AccountRestrictionMessageUpload},
	{method: http.MethodPost, path: "/api/v1/track/create", message: service.AccountRestrictionMessageUpload},
	{method: http.MethodPut, prefix: "/api/v1/track/", suffix: "/update", message: service.AccountRestrictionMessageUpload},
	{method: http.MethodPost, prefix: "/api/v1/track/", suffix: "/upload_cloud", message: service.AccountRestrictionMessageUpload},
	{method: http.MethodPost, path: "/api/v1/companion/session/create", message: service.AccountRestrictionMessageCompanion},
	{method: http.MethodPost, prefix: "/api/v1/user/", suffix: "/follow", message: service.AccountRestrictionMessageFollow},
	{method: http.MethodPost, path: "/api/v1/track_collect", message: service.AccountRestrictionMessageCollect},
	{method: http.MethodPut, path: "/api/v1/user/profile/update", message: service.AccountRestrictionMessageProfile},
	{method: http.MethodPut, path: "/api/v1/user/profile/phone", message: service.AccountRestrictionMessageProfile},
	{method: http.MethodPut, path: "/api/v1/user/profile/client_language", message: service.AccountRestrictionMessageProfile},
}

func AccountRestrictionMiddleware(restrictionSvc *service.AccountRestrictionService) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		message, ok := accountRestrictionMessage(string(c.Method()), string(c.Path()))
		if !ok {
			c.Next(ctx)
			return
		}
		if restrictionSvc == nil {
			c.Next(ctx)
			return
		}
		meta := GetRequestMeta(c)
		if meta == nil || meta.AuthUserID <= 0 {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "unauthorized"})
			c.Abort()
			return
		}
		err := restrictionSvc.EnsureAllowed(ctx, meta.AuthUserID, message)
		if err == nil {
			c.Next(ctx)
			return
		}
		var blocked *service.AccountRestrictionBlockedError
		if errors.As(err, &blocked) {
			c.JSON(http.StatusForbidden, utils.H{
				"error":   "account restricted",
				"message": blocked.Message,
				"data":    blocked.Restriction,
			})
			c.Abort()
			return
		}
		if errors.Is(err, repository.ErrNotFound) {
			c.Next(ctx)
			return
		}
		c.JSON(http.StatusInternalServerError, utils.H{"error": err.Error()})
		c.Abort()
	}
}

func accountRestrictionMessage(method, path string) (string, bool) {
	for _, route := range accountRestrictedRoutes {
		if method != route.method {
			continue
		}
		if route.path != "" && path == route.path {
			return route.message, true
		}
		if route.prefix != "" && strings.HasPrefix(path, route.prefix) && strings.HasSuffix(path, route.suffix) {
			return route.message, true
		}
	}
	return "", false
}
