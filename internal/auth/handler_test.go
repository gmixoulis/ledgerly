package auth

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testUser = "admin"
	testPass = "password"
)

func TestHandler_Login(t *testing.T) {
	h := NewHandler(testSecret, testUser, testPass)

	t.Run("missing/bad body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader("not-json"))
		w := httptest.NewRecorder()
		h.Login(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("wrong credentials", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"username": "nope", "password": "wrong"})
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.Login(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", w.Code)
		}
	})

	t.Run("valid credentials", func(t *testing.T) {
		body, _ := json.Marshal(map[string]string{"username": testUser, "password": testPass})
		req := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.Login(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
		}

		var resp map[string]string
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		tokStr := resp["token"]
		if tokStr == "" {
			t.Fatal("expected non-empty token")
		}

		// Confirm the token actually parses with our secret.
		tok, err := jwt.Parse(tokStr, func(t *jwt.Token) (any, error) {
			return []byte(testSecret), nil
		})
		if err != nil || !tok.Valid {
			t.Fatalf("issued token did not validate: err=%v valid=%v", err, tok != nil && tok.Valid)
		}
		claims, ok := tok.Claims.(jwt.MapClaims)
		if !ok {
			t.Fatal("expected MapClaims")
		}
		if claims["sub"] != testUser {
			t.Errorf("sub: got %v, want %s", claims["sub"], testUser)
		}
	})
}
