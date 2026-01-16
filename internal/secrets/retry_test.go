package secrets

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestIsRetriableError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"not found error", status.Error(codes.NotFound, "not found"), false},
		{"permission denied", status.Error(codes.PermissionDenied, "denied"), false},
		{"unavailable", status.Error(codes.Unavailable, "unavailable"), true},
		{"resource exhausted", status.Error(codes.ResourceExhausted, "quota"), true},
		{"deadline exceeded", status.Error(codes.DeadlineExceeded, "timeout"), true},
		{"aborted", status.Error(codes.Aborted, "aborted"), true},
		{"connection error", errors.New("connection refused"), true},
		{"timeout error", errors.New("operation timeout"), true},
		{"rate limit error", errors.New("rate limit exceeded"), true},
		{"generic error", errors.New("something went wrong"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRetriableError(tt.err); got != tt.expected {
				t.Errorf("isRetriableError() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestRetry_SuccessOnFirstAttempt(t *testing.T) {
	ctx := context.Background()
	calls := 0

	result, err := retry(ctx, func() (string, error) {
		calls++
		return "success", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %s", result)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetry_SuccessAfterRetries(t *testing.T) {
	ctx := context.Background()
	calls := 0

	result, err := retry(ctx, func() (string, error) {
		calls++
		if calls < 3 {
			return "", status.Error(codes.Unavailable, "temporarily unavailable")
		}
		return "success", nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %s", result)
	}
	if calls != 3 {
		t.Errorf("expected 3 calls, got %d", calls)
	}
}

func TestRetry_NonRetriableErrorNoRetry(t *testing.T) {
	ctx := context.Background()
	calls := 0

	_, err := retry(ctx, func() (string, error) {
		calls++
		return "", status.Error(codes.NotFound, "not found")
	})

	if err == nil {
		t.Error("expected error, got nil")
	}
	if calls != 1 {
		t.Errorf("expected 1 call for non-retriable error, got %d", calls)
	}
}

func TestRetry_ExhaustsRetries(t *testing.T) {
	ctx := context.Background()
	calls := 0

	_, err := retry(ctx, func() (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "still unavailable")
	})

	if err == nil {
		t.Error("expected error after retries exhausted")
	}
	// maxRetries + 1 (initial attempt)
	expectedCalls := maxRetries + 1
	if calls != expectedCalls {
		t.Errorf("expected %d calls, got %d", expectedCalls, calls)
	}
}

func TestRetry_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := retry(ctx, func() (string, error) {
		calls++
		return "", status.Error(codes.Unavailable, "unavailable")
	})

	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRetryNoResult_Success(t *testing.T) {
	ctx := context.Background()
	calls := 0

	err := retryNoResult(ctx, func() error {
		calls++
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call, got %d", calls)
	}
}

func TestRetryNoResult_RetriesOnTransientError(t *testing.T) {
	ctx := context.Background()
	calls := 0

	err := retryNoResult(ctx, func() error {
		calls++
		if calls < 2 {
			return status.Error(codes.ResourceExhausted, "quota exceeded")
		}
		return nil
	})

	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if calls != 2 {
		t.Errorf("expected 2 calls, got %d", calls)
	}
}

func TestFormatAuthError(t *testing.T) {
	tests := []struct {
		name        string
		errMsg      string
		shouldMatch string
	}{
		{
			name:        "no credentials",
			errMsg:      "google: could not find default credentials",
			shouldMatch: "gcloud auth application-default login",
		},
		{
			name:        "token expired",
			errMsg:      "token expired and refresh failed",
			shouldMatch: "credentials expired",
		},
		{
			name:        "generic error",
			errMsg:      "some random network error",
			shouldMatch: "failed to create GCP client",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := formatAuthError(errors.New(tt.errMsg))
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.shouldMatch) {
				t.Errorf("error %q should contain %q", err.Error(), tt.shouldMatch)
			}
		})
	}
}

func TestFormatPermissionError(t *testing.T) {
	err := FormatPermissionError("my-secret", "my-project", "accessing")

	errMsg := err.Error()
	if !strings.Contains(errMsg, "my-secret") {
		t.Error("should contain secret name")
	}
	if !strings.Contains(errMsg, "my-project") {
		t.Error("should contain project name")
	}
	if !strings.Contains(errMsg, "coffer auth status") {
		t.Error("should suggest auth status command")
	}
	if !strings.Contains(errMsg, "roles/secretmanager.secretAccessor") {
		t.Error("should mention required IAM role")
	}
}
