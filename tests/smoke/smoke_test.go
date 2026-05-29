//go:build smoke

// Package smoke runs a full end-to-end lifecycle test. Run with: go test -tags=smoke ./tests/smoke/...
package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gmixoulis/company-service/internal/auth"
	"github.com/gmixoulis/company-service/internal/company"
	"github.com/gmixoulis/company-service/internal/events"
	"github.com/gmixoulis/company-service/internal/server"
)

const (
	smokeSecret = "smoke-test-secret"
	smokeUser   = "admin"
	smokePass   = "password"
	defaultDSN  = "postgres://postgres:postgres@localhost:5432/companies_test?sslmode=disable"
)

func dsn() string {
	if v := os.Getenv("TEST_DATABASE_DSN"); v != "" {
		return v
	}
	return defaultDSN
}

func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn())
	if err != nil {
		t.Skipf("skipping smoke test: cannot connect to database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping smoke test: database ping failed: %v", err)
	}

	schema := `
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
		DO $$ BEGIN
			IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'company_type') THEN
				CREATE TYPE company_type AS ENUM (
					'Corporations', 'NonProfit', 'Cooperative', 'Sole Proprietorship'
				);
			END IF;
		END $$;
		CREATE TABLE IF NOT EXISTS companies (
			id               UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
			name             VARCHAR(15)  NOT NULL UNIQUE,
			description      VARCHAR(3000),
			amount_employees INT          NOT NULL CHECK (amount_employees >= 0),
			registered       BOOLEAN      NOT NULL DEFAULT FALSE,
			type             company_type NOT NULL,
			created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
			updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
		);`

	if _, err := pool.Exec(context.Background(), schema); err != nil {
		pool.Close()
		t.Fatalf("setup schema: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), "TRUNCATE TABLE companies")
		pool.Close()
	})
	return pool
}

func newServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()
	repo := company.NewRepository(pool)
	svc := company.NewService(repo, events.NopProducer{})
	companyHandler := company.NewHandler(svc)
	authHandler := auth.NewHandler(smokeSecret, smokeUser, smokePass)
	authMiddleware := auth.NewMiddleware(smokeSecret)
	return httptest.NewServer(server.New(companyHandler, authHandler, authMiddleware))
}

func mustJSON(t *testing.T, v any) *bytes.Reader {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return bytes.NewReader(b)
}

func TestSmoke_EndToEnd(t *testing.T) {
	pool := setupDB(t)
	srv := newServer(t, pool)
	defer srv.Close()

	client := srv.Client()

	loginResp, err := client.Post(srv.URL+"/auth/login", "application/json",
		mustJSON(t, map[string]string{"username": smokeUser, "password": smokePass}))
	if err != nil {
		t.Fatalf("login request: %v", err)
	}
	defer loginResp.Body.Close()
	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", loginResp.StatusCode)
	}
	var loginBody map[string]string
	if err := json.NewDecoder(loginResp.Body).Decode(&loginBody); err != nil {
		t.Fatalf("decode login: %v", err)
	}
	token := loginBody["token"]
	if token == "" {
		t.Fatal("expected non-empty token from login")
	}

	authReq := func(method, path string, body any) *http.Request {
		t.Helper()
		var rdr *bytes.Reader
		if body != nil {
			rdr = mustJSON(t, body)
		} else {
			rdr = bytes.NewReader(nil)
		}
		req, err := http.NewRequest(method, srv.URL+path, rdr)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	createResp, err := client.Do(authReq(http.MethodPost, "/api/v1/companies", map[string]any{
		"name":             "Smoke Inc",
		"description":      "smoke test target",
		"amount_employees": 7,
		"registered":       true,
		"type":             "Corporations",
	}))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("create: expected 201, got %d", createResp.StatusCode)
	}
	var created company.Company
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.ID.String() == "" {
		t.Fatal("expected non-empty ID")
	}

	companyPath := fmt.Sprintf("/api/v1/companies/%s", created.ID)

	getResp, err := client.Get(srv.URL + companyPath)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("get: expected 200, got %d", getResp.StatusCode)
	}

	patchResp, err := client.Do(authReq(http.MethodPatch, companyPath, map[string]any{
		"amount_employees": 42,
	}))
	if err != nil {
		t.Fatalf("patch: %v", err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("patch: expected 200, got %d", patchResp.StatusCode)
	}
	var patched company.Company
	_ = json.NewDecoder(patchResp.Body).Decode(&patched)
	if patched.AmountEmployees != 42 {
		t.Fatalf("patch: expected employees=42, got %d", patched.AmountEmployees)
	}

	delResp, err := client.Do(authReq(http.MethodDelete, companyPath, nil))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", delResp.StatusCode)
	}

	gone, err := client.Get(srv.URL + companyPath)
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	defer gone.Body.Close()
	if gone.StatusCode != http.StatusNotFound {
		t.Fatalf("get after delete: expected 404, got %d", gone.StatusCode)
	}
}
