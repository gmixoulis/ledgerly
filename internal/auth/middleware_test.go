package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "unit-test-secret"

// recorderHandler records whether it was called and what claims sat on the context.
type recorderHandler struct {
	called bool
	claims jwt.Claims
}

func (rh *recorderHandler) ServeHTTP(_ http.ResponseWriter, r *http.Request) {
	rh.called = true
	if c, ok := r.Context().Value(ClaimsKey).(jwt.Claims); ok {
		rh.claims = c
	}
}

func signToken(t *testing.T, method jwt.SigningMethod, key any, claims jwt.MapClaims) string {
	t.Helper()
	tok, err := jwt.NewWithClaims(method, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func TestMiddleware_Authenticate(t *testing.T) {
	mw := NewMiddleware(testSecret)

	tests := []struct {
		name       string
		header     string
		wantStatus int
		wantCalled bool
	}{
		{
			name:       "missing header",
			header:     "",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "malformed header (no scheme)",
			header:     "abc.def.ghi",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name:       "malformed header (wrong scheme)",
			header:     "Basic abc",
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name: "expired token",
			header: "Bearer " + signToken(t,
				jwt.SigningMethodHS256, []byte(testSecret),
				jwt.MapClaims{"sub": "u", "exp": time.Now().Add(-time.Hour).Unix()}),
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name: "wrong signing method (none)",
			header: "Bearer " + signToken(t,
				jwt.SigningMethodNone, jwt.UnsafeAllowNoneSignatureType,
				jwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()}),
			wantStatus: http.StatusUnauthorized,
			wantCalled: false,
		},
		{
			name: "valid token",
			header: "Bearer " + signToken(t,
				jwt.SigningMethodHS256, []byte(testSecret),
				jwt.MapClaims{"sub": "u", "exp": time.Now().Add(time.Hour).Unix()}),
			wantStatus: http.StatusOK,
			wantCalled: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			next := &recorderHandler{}
			h := mw.Authenticate(next)

			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != tc.wantStatus {
				t.Errorf("status: got %d, want %d", w.Code, tc.wantStatus)
			}
			if next.called != tc.wantCalled {
				t.Errorf("next called: got %v, want %v", next.called, tc.wantCalled)
			}
			if tc.wantCalled && next.claims == nil {
				t.Error("expected claims on context, got nil")
			}
		})
	}
}
