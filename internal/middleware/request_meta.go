package middleware

import (
	"context"
	"strconv"

	"github.com/cloudwego/hertz/pkg/app"

	"github.com/tongyichu/track_server/internal/models"
)

// Header keys expected from client.
const (
	HeaderUserID         = "X-User-ID"         //用户ID
	HeaderClientVersion  = "X-Client-Version"  // 客户端版本
	HeaderClientLanguage = "X-Client-Language" // 客户端语言
	HeaderLocation       = "X-User-Location"   // 地理位置
	HeaderPlatform       = "X-Platform"        // ios or android
	HeaderDeviceID       = "X-Device-ID"       // 设备ID

	// CtxRequestHeaderMetaKey is the context key used to store RequestMeta on RequestContext.
	CtxRequestHeaderMetaKey = "request_header_meta"
)

// RequestMetaMiddleware extracts required header fields and stores them into RequestContext.
func RequestMetaMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		rawUserID := string(c.Request.Header.Peek(HeaderUserID))
		meta := &models.RequestMeta{
			RawUserID:      rawUserID,
			ClientVersion:  string(c.Request.Header.Peek(HeaderClientVersion)),
			ClientLanguage: string(c.Request.Header.Peek(HeaderClientLanguage)),
			Location:       string(c.Request.Header.Peek(HeaderLocation)),
			Platform:       string(c.Request.Header.Peek(HeaderPlatform)),
			DeviceID:       string(c.Request.Header.Peek(HeaderDeviceID)),
		}
		if rawUserID != "" {
			if userID, err := strconv.ParseInt(rawUserID, 10, 64); err == nil && userID > 0 {
				meta.UserID = userID
			}
		}
		c.Set(CtxRequestHeaderMetaKey, meta)
	}
}

// GetRequestMeta retrieves RequestMeta from RequestContext if present.
func GetRequestMeta(c *app.RequestContext) *models.RequestMeta {
	v, ok := c.Get(CtxRequestHeaderMetaKey)
	if !ok {
		return nil
	}
	if meta, ok := v.(*models.RequestMeta); ok {
		return meta
	}
	return nil
}
