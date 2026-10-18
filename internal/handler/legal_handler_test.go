package handler_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestLegalPagesPublic(t *testing.T) {
	e := newTestEnv()
	defer e.close()

	for _, page := range []struct {
		path  string
		title string
	}{
		{"/api/v1/legal/privacy-policy.html", "隐私政策"},
		{"/api/v1/legal/terms-of-service.html", "用户服务协议"},
	} {
		for _, query := range []string{"", "?is_dark=false", "?lang=english&is_dark=true"} {
			t.Run(page.path+query, func(t *testing.T) {
				w := e.perform(http.MethodGet, page.path+query, nil)
				resp := w.Result()
				if resp.StatusCode() != http.StatusOK {
					t.Fatalf("public page status=%d body=%s", resp.StatusCode(), resp.Body())
				}
				if contentType := string(resp.Header.Get("Content-Type")); contentType != "text/html; charset=utf-8" {
					t.Fatalf("unexpected content type %q", contentType)
				}
				if body := string(resp.Body()); !strings.Contains(body, page.title) || !strings.HasPrefix(body, "<!doctype html>") {
					t.Fatalf("missing HTML document or title %q", page.title)
				}
			})
		}
	}
}
