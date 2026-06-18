package handler_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tongyichu/track_server/internal/models"
)

func TestAccountRestrictionMiddlewareBlocksRestrictedActions(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	ensureTestUser(t, e, 1001, "restricted")
	token := e.generateTestToken(1001)

	if err := e.restrictionRepo.CreateAccountRestriction(context.Background(), &models.AccountRestriction{
		UserID:    1001,
		Status:    models.AccountRestrictionStatusActive,
		Reason:    "违规上传内容",
		Operator:  "ops",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create restriction: %v", err)
	}

	w := e.perform(http.MethodPost, "/api/v1/track_collect?track_id=NO.00000001", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected restricted collect status 403, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	if body := string(w.Body.Bytes()); !strings.Contains(body, "账号已被限制，禁止收藏轨迹") {
		t.Fatalf("expected collect restriction message, got %s", body)
	}

	w = e.perform(http.MethodPost, "/api/v1/track/create", []byte(`{"title":"blocked"}`), authHeader(token))
	if w.Result().StatusCode() != http.StatusForbidden {
		t.Fatalf("expected restricted track create status 403, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
	if body := string(w.Body.Bytes()); !strings.Contains(body, "账号已被限制，禁止上传内容") {
		t.Fatalf("expected upload restriction message, got %s", body)
	}
}

func TestAccountRestrictionMiddlewareAllowsUnrestrictingActions(t *testing.T) {
	e := newTestEnv()
	defer e.close()
	ensureTestUser(t, e, 1001, "restricted")
	token := e.generateTestToken(1001)

	if err := e.restrictionRepo.CreateAccountRestriction(context.Background(), &models.AccountRestriction{
		UserID:    1001,
		Status:    models.AccountRestrictionStatusActive,
		Reason:    "违规上传内容",
		Operator:  "ops",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("create restriction: %v", err)
	}

	w := e.perform(http.MethodDelete, "/api/v1/track_collect?track_id=NO.00000001", nil, authHeader(token))
	if w.Result().StatusCode() != http.StatusOK {
		t.Fatalf("expected uncollect status 200, got %d body=%s", w.Result().StatusCode(), string(w.Body.Bytes()))
	}
}
