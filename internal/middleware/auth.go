package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/golang-jwt/jwt/v5"
	"github.com/tongyichu/track_server/internal/service"
)

func JWTAuthMiddleware(loginSvc *service.LoginService, blacklist *TokenBlacklist) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if loginSvc == nil {
			c.JSON(http.StatusInternalServerError, utils.H{"error": "login service not configured"})
			c.Abort()
			return
		}
		authHeader := string(c.Request.Header.Peek("Authorization"))
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenStr == authHeader {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "invalid authorization format, expected: Bearer <token>"})
			c.Abort()
			return
		}

		if blacklist != nil && blacklist.IsBlacklisted(tokenStr) {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "token has been revoked"})
			c.Abort()
			return
		}

		token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(loginSvc.JWTSecret()), nil
		})
		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		var userID int64
		switch v := claims["user_id"].(type) {
		case float64:
			userID = int64(v)
		case string:
			userID, _ = strconv.ParseInt(v, 10, 64)
		}

		if userID <= 0 {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "invalid user_id in token"})
			c.Abort()
			return
		}

		tokenVersion := int64(1)
		switch v := claims["token_version"].(type) {
		case float64:
			tokenVersion = int64(v)
		case string:
			if parsed, err := strconv.ParseInt(v, 10, 64); err == nil && parsed > 0 {
				tokenVersion = parsed
			}
		}

		currentVersion, err := loginSvc.GetUserTokenVersion(ctx, userID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}
		if currentVersion != tokenVersion {
			c.JSON(http.StatusUnauthorized, utils.H{"error": "token has been revoked"})
			c.Abort()
			return
		}

		var expiresAt time.Time
		if exp, ok := claims["exp"].(float64); ok {
			expiresAt = time.Unix(int64(exp), 0)
		}
		shouldRenew := !expiresAt.IsZero() && time.Until(expiresAt) <= loginSvc.RenewWindow()

		meta := GetRequestMeta(c)
		if meta != nil {
			meta.AuthUserID = userID
			meta.RawUserID = strconv.FormatInt(userID, 10)
		}

		c.Next(ctx)

		if string(c.Path()) == "/api/v1/logout" || c.Response.StatusCode() >= http.StatusBadRequest || !shouldRenew {
			return
		}
		latestVersion, err := loginSvc.GetUserTokenVersion(ctx, userID)
		if err != nil || latestVersion != tokenVersion {
			return
		}
		renewedToken, err := loginSvc.GenerateToken(userID, tokenVersion)
		if err != nil {
			return
		}
		c.Response.Header.Set("X-Renewed-Token", renewedToken)
	}
}
