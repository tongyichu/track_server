package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tongyichu/track_server/internal/ai"
	"io"
	"net/http"
	"strings"
	"time"
)

var (
	errKimiInsufficientBalance = errors.New("Kimi API 账户余额不足，请在开放平台补充余额后重试；原说明未修改")
	errKimiSearchDenied        = errors.New("Kimi 联网检索被服务商拒绝（403），请检查 API 账户状态和搜索服务权限；原说明未修改")
)

type kimiHTTPError struct {
	StatusCode int
	Parameter  string
}

func (e *kimiHTTPError) Error() string {
	if e.Parameter != "" {
		return fmt.Sprintf("Kimi HTTP %d (parameter=%s)", e.StatusCode, e.Parameter)
	}
	return fmt.Sprintf("Kimi HTTP %d", e.StatusCode)
}

func kimiResponseError(response *http.Response) error {
	var failure struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	result := &kimiHTTPError{StatusCode: response.StatusCode}
	if json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&failure) != nil {
		return result
	}
	message := strings.ToLower(failure.Error.Message)
	if failure.Error.Type == "exceeded_current_quota_error" || strings.Contains(message, "insufficient balance") {
		return errKimiInsufficientBalance
	}
	// Only known categories leave this boundary; provider errors may echo credentials or input.
	for _, parameter := range []string{"temperature", "thinking", "max_tokens", "image"} {
		if strings.Contains(message, parameter) {
			result.Parameter = parameter
			break
		}
	}
	return result
}

func kimiBalanceInsufficient(ctx context.Context, client *http.Client, key string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, kimiAPIBaseURL+"/users/me/balance", nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("Authorization", "Bearer "+key)
	response, err := ai.Observe(ctx, client, request, "balance")
	if err != nil {
		return false, errors.New("Kimi balance check failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, kimiResponseError(response)
	}
	var balance struct {
		Code int `json:"code"`
		Data *struct {
			Available *float64 `json:"available_balance"`
		} `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 16<<10)).Decode(&balance); err != nil || balance.Code != 0 || balance.Data == nil || balance.Data.Available == nil {
		return false, errors.New("invalid Kimi balance response")
	}
	return *balance.Data.Available <= 0, nil
}
