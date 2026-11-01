package middleware

import (
	"context"
	"strconv"
	"strings"

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
	HeaderDeviceModel    = "X-Device-Model"    // 设备型号
	HeaderOSVersion      = "X-OS-Version"      // 系统版本
	HeaderVersionCode    = "X-Version-Code"    // 客户端数字版本号

	// CtxRequestHeaderMetaKey is the context key used to store RequestMeta on RequestContext.
	CtxRequestHeaderMetaKey = "request_header_meta"
)

// RequestMetaMiddleware extracts required header fields and stores them into RequestContext.
func RequestMetaMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		headerUserID := string(c.Request.Header.Peek(HeaderUserID))
		versionCode, _ := strconv.ParseInt(strings.TrimSpace(string(c.Request.Header.Peek(HeaderVersionCode))), 10, 64)
		meta := &models.RequestMeta{
			UserID:         headerUserID,
			ClientVersion:  string(c.Request.Header.Peek(HeaderClientVersion)),
			ClientLanguage: string(c.Request.Header.Peek(HeaderClientLanguage)),
			Location:       string(c.Request.Header.Peek(HeaderLocation)),
			Platform:       string(c.Request.Header.Peek(HeaderPlatform)),
			DeviceID:       string(c.Request.Header.Peek(HeaderDeviceID)),
			DeviceModel:    string(c.Request.Header.Peek(HeaderDeviceModel)),
			OSVersion:      string(c.Request.Header.Peek(HeaderOSVersion)),
			VersionCode:    versionCode,
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
