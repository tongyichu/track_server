package middleware

import (
	"context"

	"github.com/cloudwego/hertz/pkg/app"

	"trackapp-server/internal/models"
)

// Header keys expected from client.
const (
	HeaderUserID         = "X-User-ID"
	HeaderClientType     = "X-Client-Type"
	HeaderClientVersion  = "X-Client-Version"
	HeaderClientLanguage = "X-Client-Language"
	HeaderLocation       = "X-User-Location"

	// CtxRequestMetaKey is the context key used to store RequestMeta on RequestContext.
	CtxRequestMetaKey = "request_meta"
)

// RequestMetaMiddleware extracts required header fields and stores them into RequestContext.
func RequestMetaMiddleware() app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		meta := &models.RequestMeta{
			UserID:         string(c.Request.Header.Peek(HeaderUserID)),
			ClientType:     string(c.Request.Header.Peek(HeaderClientType)),
			ClientVersion:  string(c.Request.Header.Peek(HeaderClientVersion)),
			ClientLanguage: string(c.Request.Header.Peek(HeaderClientLanguage)),
			Location:       string(c.Request.Header.Peek(HeaderLocation)),
		}
		c.Set(CtxRequestMetaKey, meta)
	}
}

// GetRequestMeta retrieves RequestMeta from RequestContext if present.
func GetRequestMeta(c *app.RequestContext) *models.RequestMeta {
	v, ok := c.Get(CtxRequestMetaKey)
	if !ok {
		return nil
	}
	if meta, ok := v.(*models.RequestMeta); ok {
		return meta
	}
	return nil
}
