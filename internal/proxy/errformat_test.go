package proxy

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
)

func TestGenericErrorFormatter(t *testing.T) {
	f := &GenericErrorFormatter{}
	rec := httptest.NewRecorder()
	f.WriteError(rec, 403, "forbidden path")

	if rec.Code != 403 {
		t.Errorf("code = %d, want 403", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	errObj := resp["error"].(map[string]interface{})

	if errObj["source"] != "vibeproxy" {
		t.Errorf("source = %v, want vibeproxy", errObj["source"])
	}
	if errObj["message"] != "forbidden path" {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestOpenAIErrorFormatter(t *testing.T) {
	f := &OpenAIErrorFormatter{}
	rec := httptest.NewRecorder()
	f.WriteError(rec, 401, "invalid token")

	if rec.Code != 401 {
		t.Errorf("code = %d, want 401", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	errObj := resp["error"].(map[string]interface{})

	if errObj["type"] != "authentication_error" {
		t.Errorf("type = %v, want authentication_error", errObj["type"])
	}
	if errObj["code"] != "invalid_api_key" {
		t.Errorf("code = %v, want invalid_api_key", errObj["code"])
	}
	if errObj["param"] != nil {
		t.Errorf("param = %v, want nil", errObj["param"])
	}
	if errObj["message"] != "invalid token" {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestOpenAIErrorFormatter_RateLimit(t *testing.T) {
	f := &OpenAIErrorFormatter{}
	rec := httptest.NewRecorder()
	f.WriteError(rec, 429, "rate limit exceeded")

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	errObj := resp["error"].(map[string]interface{})

	if errObj["type"] != "rate_limit_error" {
		t.Errorf("type = %v, want rate_limit_error", errObj["type"])
	}
}

func TestAnthropicErrorFormatter(t *testing.T) {
	f := &AnthropicErrorFormatter{}
	rec := httptest.NewRecorder()
	f.WriteError(rec, 401, "invalid token")

	if rec.Code != 401 {
		t.Errorf("code = %d, want 401", rec.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["type"] != "error" {
		t.Errorf("top-level type = %v, want error", resp["type"])
	}
	errObj := resp["error"].(map[string]interface{})

	if errObj["type"] != "authentication_error" {
		t.Errorf("error.type = %v, want authentication_error", errObj["type"])
	}
	if errObj["message"] != "invalid token" {
		t.Errorf("message = %v", errObj["message"])
	}
}

func TestAnthropicErrorFormatter_RateLimit(t *testing.T) {
	f := &AnthropicErrorFormatter{}
	rec := httptest.NewRecorder()
	f.WriteError(rec, 429, "rate limited")

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	errObj := resp["error"].(map[string]interface{})

	if errObj["type"] != "rate_limit_error" {
		t.Errorf("error.type = %v, want rate_limit_error", errObj["type"])
	}
}

func TestFormatterForService(t *testing.T) {
	tests := []struct {
		service  string
		wantType string
	}{
		{"openai", "*proxy.OpenAIErrorFormatter"},
		{"anthropic", "*proxy.AnthropicErrorFormatter"},
		{"unknown", "*proxy.GenericErrorFormatter"},
	}
	for _, tt := range tests {
		f := FormatterForService(tt.service)
		got := fmt.Sprintf("%T", f)
		if got != tt.wantType {
			t.Errorf("FormatterForService(%q) = %s, want %s", tt.service, got, tt.wantType)
		}
	}
}

func TestOpenAIProxyError(t *testing.T) {
	f := &OpenAIErrorFormatter{}
	rec := httptest.NewRecorder()
	f.WriteProxyError(rec, 502, "upstream error", "connection error")

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)
	errObj := resp["error"].(map[string]interface{})

	if errObj["type"] != "server_error" {
		t.Errorf("type = %v, want server_error", errObj["type"])
	}
	if errObj["code"] != "proxy_error" {
		t.Errorf("code = %v, want proxy_error", errObj["code"])
	}
}

func TestAnthropicProxyError(t *testing.T) {
	f := &AnthropicErrorFormatter{}
	rec := httptest.NewRecorder()
	f.WriteProxyError(rec, 502, "upstream error", "timeout")

	var resp map[string]interface{}
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp["type"] != "error" {
		t.Errorf("type = %v, want error", resp["type"])
	}
	errObj := resp["error"].(map[string]interface{})
	if errObj["type"] != "api_error" {
		t.Errorf("error.type = %v, want api_error", errObj["type"])
	}
}
