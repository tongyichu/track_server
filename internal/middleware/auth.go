package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/golang-jwt/jwt/v5"
)

func JWTAuthMiddleware(secret string, blacklist *TokenBlacklist) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
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
			return []byte(secret), nil
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

		meta := GetRequestMeta(c)
		if meta != nil {
			meta.UserID = userID
			meta.RawUserID = strconv.FormatInt(userID, 10)
		}

		c.Next(ctx)
	}
}
