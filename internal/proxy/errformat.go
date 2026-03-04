package proxy

import (
	"encoding/json"
	"net/http"
)

// ErrorFormatter writes error responses in a provider-specific format.
type ErrorFormatter interface {
	WriteError(w http.ResponseWriter, code int, msg string)
	WriteProxyError(w http.ResponseWriter, code int, msg string, detail string)
}

// GenericErrorFormatter produces the legacy vibeproxy error format (fallback).
type GenericErrorFormatter struct{}

func (f *GenericErrorFormatter) WriteError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"code":    code,
			"source":  "vibeproxy",
		},
	})
}

func (f *GenericErrorFormatter) WriteProxyError(w http.ResponseWriter, code int, msg string, detail string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"code":    code,
			"source":  "upstream",
			"detail":  detail,
		},
	})
}

// OpenAIErrorFormatter produces errors matching the OpenAI SDK format:
// {"error":{"message":"...","type":"...","param":null,"code":"..."}}
type OpenAIErrorFormatter struct{}

func (f *OpenAIErrorFormatter) WriteError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": msg,
			"type":    openaiErrorType(code),
			"param":   nil,
			"code":    openaiErrorCode(code),
		},
	})
}

func (f *OpenAIErrorFormatter) WriteProxyError(w http.ResponseWriter, code int, msg string, detail string) {
	fullMsg := msg
	if detail != "" {
		fullMsg = msg + ": " + detail
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]interface{}{
			"message": fullMsg,
			"type":    "server_error",
			"param":   nil,
			"code":    "proxy_error",
		},
	})
}

// AnthropicErrorFormatter produces errors matching the Anthropic SDK format:
// {"type":"error","error":{"type":"...","message":"..."}}
type AnthropicErrorFormatter struct{}

func (f *AnthropicErrorFormatter) WriteError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    anthropicErrorType(code),
			"message": msg,
		},
	})
}

func (f *AnthropicErrorFormatter) WriteProxyError(w http.ResponseWriter, code int, msg string, detail string) {
	fullMsg := msg
	if detail != "" {
		fullMsg = msg + ": " + detail
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": fullMsg,
		},
	})
}

// FormatterForService returns the appropriate ErrorFormatter based on service name.
func FormatterForService(serviceName string) ErrorFormatter {
	switch serviceName {
	case "openai":
		return &OpenAIErrorFormatter{}
	case "anthropic":
		return &AnthropicErrorFormatter{}
	default:
		return &GenericErrorFormatter{}
	}
}

func openaiErrorType(code int) string {
	switch {
	case code == 401:
		return "authentication_error"
	case code == 403:
		return "permission_error"
	case code == 429:
		return "rate_limit_error"
	case code >= 500:
		return "server_error"
	default:
		return "invalid_request_error"
	}
}

func openaiErrorCode(code int) string {
	switch code {
	case 401:
		return "invalid_api_key"
	case 403:
		return "forbidden"
	case 429:
		return "rate_limit_exceeded"
	case 500, 502:
		return "server_error"
	default:
		return "invalid_request"
	}
}

func anthropicErrorType(code int) string {
	switch {
	case code == 401:
		return "authentication_error"
	case code == 403:
		return "permission_error"
	case code == 429:
		return "rate_limit_error"
	case code >= 500:
		return "api_error"
	default:
		return "invalid_request_error"
	}
}
