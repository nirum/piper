package provider

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError is a structured error returned by provider API calls.
// It carries the HTTP status code and provider-specific error details.
type APIError struct {
	// StatusCode is the HTTP response status code.
	StatusCode int
	// ErrType is the provider's error type string (e.g. "rate_limit_error").
	ErrType string
	// Message is the human-readable error message from the provider.
	Message string
	// Provider identifies which provider returned the error ("anthropic", "openai").
	Provider string
}

func (e *APIError) Error() string {
	if e.ErrType != "" && e.Message != "" {
		return fmt.Sprintf("%s API error (%d): %s: %s", e.Provider, e.StatusCode, e.ErrType, e.Message)
	}
	if e.Message != "" {
		return fmt.Sprintf("%s API error (%d): %s", e.Provider, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("%s API error (%d)", e.Provider, e.StatusCode)
}

// IsAuthError reports whether err is an API authentication/authorization error
// (HTTP 401 or 403).
func IsAuthError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusUnauthorized ||
			apiErr.StatusCode == http.StatusForbidden
	}
	return false
}

// IsRateLimitError reports whether err is an API rate-limit error (HTTP 429).
func IsRateLimitError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusTooManyRequests
	}
	return false
}
