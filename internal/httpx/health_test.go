package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubPinger struct{ err error }

func (s stubPinger) Ping(context.Context) error { return s.err }

// The test the setup ticket asks you to run. It needs no Postgres, on purpose —
// see the note on Pinger.
func TestHealthzReportsTheDatabase(t *testing.T) {
	rec := httptest.NewRecorder()
	Health(stubPinger{}, stubPinger{})(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON (%v): %s", err, rec.Body.String())
	}
	if got["status"] != "ok" {
		t.Errorf("got %v, want status=ok", got)
	}
}

// The degraded path is what proves the check reads its dependencies at all
// rather than returning a constant. Redis is failed here and Postgres is not,
// so it also proves the two are reported SEPARATELY — a single collapsed
// boolean would pass a test that only ever fails one of them.
func TestHealthzNamesWhichDependencyIsDown(t *testing.T) {
	rec := httptest.NewRecorder()
	Health(stubPinger{}, stubPinger{err: errors.New("connection refused")})(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("body is not JSON: %s", rec.Body.String())
	}
	if got["status"] != "degraded" {
		t.Errorf("status = %q, want degraded", got["status"])
	}
	if got["database"] != "ok" {
		t.Errorf("database = %q, want ok — only redis was failed", got["database"])
	}
	if got["redis"] == "ok" {
		t.Error("redis reported ok while its ping was failing")
	}
}
