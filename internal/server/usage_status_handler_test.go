package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/0xfig-labs/thinroute/internal/usage"
)

type fakeUsageSummarizer struct {
	summary   *usage.UsageSummary
	err       error
	gotParams usage.UsageQueryParams
}

func (f *fakeUsageSummarizer) GetSummary(_ context.Context, params usage.UsageQueryParams) (*usage.UsageSummary, error) {
	f.gotParams = params
	return f.summary, f.err
}

func getUsageStatus(t *testing.T, cfg *Config, target string, headers map[string]string) (*httptest.ResponseRecorder, usageStatusResponse) {
	t.Helper()
	srv := New(&mockProvider{}, cfg)
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var body usageStatusResponse
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v (body: %s)", err, rec.Body.String())
		}
	}
	return rec, body
}

func TestUsageStatusReportsManagedKeyPath(t *testing.T) {
	summarizer := &fakeUsageSummarizer{summary: &usage.UsageSummary{TotalRequests: 7, TotalTokens: 1234}}
	rec, body := getUsageStatus(t, &Config{UsageSummarizer: summarizer}, "/v1/usage", map[string]string{
		"X-thinroute-User-Path": "/team/alice",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if body.UserPath != "/team/alice" {
		t.Fatalf("user_path = %q, want /team/alice", body.UserPath)
	}
	if summarizer.gotParams.UserPath != "/team/alice" {
		t.Fatalf("summarizer path = %q, want /team/alice", summarizer.gotParams.UserPath)
	}
	if body.Usage == nil || body.Usage.TotalRequests != 7 || body.Usage.TotalTokens != 1234 {
		t.Fatalf("usage = %+v, want 7 requests / 1234 tokens", body.Usage)
	}
	if window := summarizer.gotParams.EndDate.Sub(summarizer.gotParams.StartDate); window != 29*24*time.Hour {
		t.Fatalf("default window = %s, want 29 days between inclusive bounds", window)
	}
	if body.RateLimits == nil || len(body.RateLimits) != 0 {
		t.Fatalf("rate_limits = %v, want empty array", body.RateLimits)
	}
}

func TestUsageStatusDerivedFields(t *testing.T) {
	rec, body := getUsageStatus(t, &Config{}, "/v1/usage", map[string]string{
		"X-thinroute-User-Path": "/",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if body.RateLimits == nil || len(body.RateLimits) != 0 {
		t.Fatalf("rate_limits = %v, want empty array", body.RateLimits)
	}
}

func TestUsageStatusWithoutDependenciesReturnsEmptyStatus(t *testing.T) {
	rec, body := getUsageStatus(t, &Config{}, "/v1/usage", map[string]string{
		"X-thinroute-User-Path": "/",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if body.UserPath != "/" {
		t.Fatalf("user_path = %q, want /", body.UserPath)
	}
	if body.Usage != nil {
		t.Fatalf("usage = %+v, want null without a summarizer", body.Usage)
	}
	if body.RateLimits == nil || len(body.RateLimits) != 0 {
		t.Fatalf("rate_limits = %v, want empty array", body.RateLimits)
	}
}

func TestUsageStatusRequiresAuth(t *testing.T) {
	// Without a user-path header or auth middleware, the endpoint returns 400 (bad request).
	rec, _ := getUsageStatus(t, &Config{}, "/v1/usage", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestUsageStatusRejectsInvalidDates(t *testing.T) {
	for name, target := range map[string]string{
		"malformed start":       "/v1/usage?start_date=garbage",
		"inverted range":        "/v1/usage?start_date=2026-07-06&end_date=2026-07-01",
		"range beyond 365 days": "/v1/usage?start_date=2020-01-01&end_date=2026-01-01",
		"malformed days":        "/v1/usage?days=abc",
		"non-positive days":     "/v1/usage?days=-5",
	} {
		t.Run(name, func(t *testing.T) {
			rec, _ := getUsageStatus(t, &Config{}, target, nil)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestUsageStatusUsesDefaultWindow(t *testing.T) {
	summarizer := &fakeUsageSummarizer{summary: &usage.UsageSummary{}}
	rec, _ := getUsageStatus(t, &Config{UsageSummarizer: summarizer}, "/v1/usage", map[string]string{
		"X-thinroute-User-Path": "/",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	if window := summarizer.gotParams.EndDate.Sub(summarizer.gotParams.StartDate); window != 29*24*time.Hour {
		t.Fatalf("window = %s, want 29 days (default window)", window)
	}
}

func TestUsageStatusRejectsInvalidUserPathHeader(t *testing.T) {
	rec, _ := getUsageStatus(t, &Config{}, "/v1/usage", map[string]string{
		"X-thinroute-User-Path": "/team/../secrets",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestUsageStatusSummaryErrorSurfacesAs503(t *testing.T) {
	summarizer := &fakeUsageSummarizer{err: errors.New("query failed")}
	rec, _ := getUsageStatus(t, &Config{UsageSummarizer: summarizer}, "/v1/usage", map[string]string{
		"X-thinroute-User-Path": "/",
	})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 (body: %s)", rec.Code, rec.Body.String())
	}
}
