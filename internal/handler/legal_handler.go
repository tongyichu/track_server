package handler

import (
	"context"
	_ "embed"
	"net/http"

	"github.com/cloudwego/hertz/pkg/app"
)

//go:embed static/privacy_policy.html
var privacyPolicyHTML []byte

//go:embed static/terms_of_service.html
var termsOfServiceHTML []byte

func PrivacyPolicyPage(_ context.Context, c *app.RequestContext) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", privacyPolicyHTML)
}

func TermsOfServicePage(_ context.Context, c *app.RequestContext) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", termsOfServiceHTML)
}
