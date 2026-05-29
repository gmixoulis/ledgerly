package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gmixoulis/company-service/internal/auth"
	"github.com/gmixoulis/company-service/internal/company"
	"github.com/gmixoulis/company-service/internal/events"
	"github.com/gmixoulis/company-service/internal/server"
)

const (
	testSecret = "integration-test-secret"
	testDSN    = "postgres://postgres:postgres@localhost:5432/companies_test?sslmode=disable"
)

func dsn() string {
	if v := os.Getenv("TEST_DATABASE_DSN"); v != "" {
		return v
	}
	return testDSN
}

// setupDB connects to Postgres, runs the schema, and returns a cleanup func.
func setupDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool, err := pgxpool.New(context.Background(), dsn())
	if err != nil {
		t.Skipf("skipping integration test: cannot connect to database: %v", err)
	}
	if err := pool.Ping(context.Background()); err != nil {
		pool.Close()
		t.Skipf("skipping integration test: database ping failed: %v", err)
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

func newTestServer(t *testing.T, pool *pgxpool.Pool) *httptest.Server {
	t.Helper()

	repo := company.NewRepository(pool)
	svc := company.NewService(repo, events.NopProducer{})
	companyHandler := company.NewHandler(svc)
	authHandler := auth.NewHandler(testSecret, "admin", "password")
	authMiddleware := auth.NewMiddleware(testSecret)

	handler := server.New(companyHandler, authHandler, authMiddleware)
	return httptest.NewServer(handler)
}

func validToken(t *testing.T) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub": "admin",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func post(t *testing.T, srv *httptest.Server, path, token string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", path, err)
	}
	return resp
}

func patch(t *testing.T, srv *httptest.Server, path, token string, body any) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH %s: %v", path, err)
	}
	return resp
}

func del(t *testing.T, srv *httptest.Server, path, token string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodDelete, srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE %s: %v", path, err)
	}
	return resp
}

func get(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	return resp
}

func TestCreateCompany(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	token := validToken(t)

	resp := post(t, srv, "/api/v1/companies", token, map[string]any{
		"name":             "Acme Corp",
		"description":      "A test company",
		"amount_employees": 50,
		"registered":       true,
		"type":             "Corporations",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var c company.Company
	if err := json.NewDecoder(resp.Body).Decode(&c); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if c.ID.String() == "" {
		t.Fatal("expected non-empty id")
	}
	if c.Name != "Acme Corp" {
		t.Fatalf("expected name 'Acme Corp', got %q", c.Name)
	}
}

func TestCreateCompany_RequiresAuth(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	resp := post(t, srv, "/api/v1/companies", "", map[string]any{
		"name":             "No Auth Co",
		"amount_employees": 1,
		"registered":       false,
		"type":             "NonProfit",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestCreateCompany_DuplicateName(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	token := validToken(t)
	body := map[string]any{
		"name":             "UniqueOne",
		"amount_employees": 10,
		"registered":       false,
		"type":             "Cooperative",
	}

	post(t, srv, "/api/v1/companies", token, body)
	resp := post(t, srv, "/api/v1/companies", token, body)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409, got %d", resp.StatusCode)
	}
}

func TestGetCompany(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	token := validToken(t)

	createResp := post(t, srv, "/api/v1/companies", token, map[string]any{
		"name":             "GetMe",
		"amount_employees": 5,
		"registered":       false,
		"type":             "NonProfit",
	})
	defer createResp.Body.Close()

	var created company.Company
	_ = json.NewDecoder(createResp.Body).Decode(&created)

	resp := get(t, srv, fmt.Sprintf("/api/v1/companies/%s", created.ID))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var fetched company.Company
	_ = json.NewDecoder(resp.Body).Decode(&fetched)

	if fetched.ID != created.ID {
		t.Fatalf("id mismatch: got %s, want %s", fetched.ID, created.ID)
	}
}

func TestGetCompany_NotFound(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	resp := get(t, srv, "/api/v1/companies/00000000-0000-0000-0000-000000000000")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestPatchCompany(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	token := validToken(t)

	createResp := post(t, srv, "/api/v1/companies", token, map[string]any{
		"name":             "PatchMe",
		"amount_employees": 10,
		"registered":       false,
		"type":             "Cooperative",
	})
	defer createResp.Body.Close()

	var created company.Company
	_ = json.NewDecoder(createResp.Body).Decode(&created)

	newEmployees := 99
	patchResp := patch(t, srv, fmt.Sprintf("/api/v1/companies/%s", created.ID), token, map[string]any{
		"amount_employees": newEmployees,
	})
	defer patchResp.Body.Close()

	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", patchResp.StatusCode)
	}

	var patched company.Company
	_ = json.NewDecoder(patchResp.Body).Decode(&patched)

	if patched.AmountEmployees != newEmployees {
		t.Fatalf("expected %d employees, got %d", newEmployees, patched.AmountEmployees)
	}
}

func TestDeleteCompany(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	token := validToken(t)

	createResp := post(t, srv, "/api/v1/companies", token, map[string]any{
		"name":             "DeleteMe",
		"amount_employees": 1,
		"registered":       false,
		"type":             "Sole Proprietorship",
	})
	defer createResp.Body.Close()

	var created company.Company
	_ = json.NewDecoder(createResp.Body).Decode(&created)

	delResp := del(t, srv, fmt.Sprintf("/api/v1/companies/%s", created.ID), token)
	defer delResp.Body.Close()

	if delResp.StatusCode != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delResp.StatusCode)
	}

	getResp := get(t, srv, fmt.Sprintf("/api/v1/companies/%s", created.ID))
	defer getResp.Body.Close()

	if getResp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 after delete, got %d", getResp.StatusCode)
	}
}

func TestLoginAndUseToken(t *testing.T) {
	pool := setupDB(t)
	srv := newTestServer(t, pool)
	defer srv.Close()

	loginResp := post(t, srv, "/auth/login", "", map[string]any{
		"username": "admin",
		"password": "password",
	})
	defer loginResp.Body.Close()

	if loginResp.StatusCode != http.StatusOK {
		t.Fatalf("login: expected 200, got %d", loginResp.StatusCode)
	}

	var tokenBody map[string]string
	_ = json.NewDecoder(loginResp.Body).Decode(&tokenBody)
	token := tokenBody["token"]

	if token == "" {
		t.Fatal("expected non-empty token")
	}

	resp := post(t, srv, "/api/v1/companies", token, map[string]any{
		"name":             "TokenTest",
		"amount_employees": 3,
		"registered":       false,
		"type":             "NonProfit",
	})
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create with real token: expected 201, got %d", resp.StatusCode)
	}
}
