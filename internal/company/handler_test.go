package company

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// mockService lets each test swap in the behavior it needs.
type mockService struct {
	createFn func(ctx context.Context, req CreateRequest) (*Company, error)
	getFn    func(ctx context.Context, id uuid.UUID) (*Company, error)
	patchFn  func(ctx context.Context, id uuid.UUID, req PatchRequest) (*Company, error)
	deleteFn func(ctx context.Context, id uuid.UUID) error
}

func (m *mockService) Create(ctx context.Context, req CreateRequest) (*Company, error) {
	return m.createFn(ctx, req)
}
func (m *mockService) GetByID(ctx context.Context, id uuid.UUID) (*Company, error) {
	return m.getFn(ctx, id)
}
func (m *mockService) Patch(ctx context.Context, id uuid.UUID, req PatchRequest) (*Company, error) {
	return m.patchFn(ctx, id, req)
}
func (m *mockService) Delete(ctx context.Context, id uuid.UUID) error {
	return m.deleteFn(ctx, id)
}

func newRouter(svc Service) *chi.Mux {
	h := NewHandler(svc)
	r := chi.NewRouter()
	r.Post("/companies", h.Create)
	r.Get("/companies/{id}", h.Get)
	r.Patch("/companies/{id}", h.Patch)
	r.Delete("/companies/{id}", h.Delete)
	return r
}

func doJSON(r http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestHandler_Create(t *testing.T) {
	t.Run("bad json", func(t *testing.T) {
		r := newRouter(&mockService{})
		req := httptest.NewRequest(http.MethodPost, "/companies", strings.NewReader("not-json"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("missing required field", func(t *testing.T) {
		r := newRouter(&mockService{})
		w := doJSON(r, http.MethodPost, "/companies", map[string]any{
			"description": "x",
		})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		svc := &mockService{
			createFn: func(_ context.Context, _ CreateRequest) (*Company, error) {
				return nil, ErrDuplicateName
			},
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodPost, "/companies", map[string]any{
			"name": "Dup", "amount_employees": 1, "registered": false, "type": "Corporations",
		})
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		want := &Company{ID: uuid.New(), Name: "OK", AmountEmployees: 1, Type: Corporations}
		svc := &mockService{
			createFn: func(_ context.Context, _ CreateRequest) (*Company, error) {
				return want, nil
			},
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodPost, "/companies", map[string]any{
			"name": "OK", "amount_employees": 1, "registered": false, "type": "Corporations",
		})
		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d (body: %s)", w.Code, w.Body.String())
		}
		var got Company
		_ = json.Unmarshal(w.Body.Bytes(), &got)
		if got.ID != want.ID {
			t.Fatalf("id mismatch")
		}
	})
}

func TestHandler_Get(t *testing.T) {
	id := uuid.New()

	t.Run("invalid id", func(t *testing.T) {
		r := newRouter(&mockService{})
		w := doJSON(r, http.MethodGet, "/companies/not-a-uuid", nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := &mockService{
			getFn: func(_ context.Context, _ uuid.UUID) (*Company, error) {
				return nil, ErrNotFound
			},
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodGet, "/companies/"+id.String(), nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc := &mockService{
			getFn: func(_ context.Context, gotID uuid.UUID) (*Company, error) {
				if gotID != id {
					t.Errorf("handler passed wrong id: got %s want %s", gotID, id)
				}
				return &Company{ID: id, Name: "Got", Type: NonProfit}, nil
			},
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodGet, "/companies/"+id.String(), nil)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

func TestHandler_Patch(t *testing.T) {
	id := uuid.New()

	t.Run("bad json", func(t *testing.T) {
		r := newRouter(&mockService{})
		req := httptest.NewRequest(http.MethodPatch, "/companies/"+id.String(), strings.NewReader("oops"))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", w.Code)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc := &mockService{
			patchFn: func(_ context.Context, _ uuid.UUID, _ PatchRequest) (*Company, error) {
				return nil, ErrNotFound
			},
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodPatch, "/companies/"+id.String(), map[string]any{
			"amount_employees": 5,
		})
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("duplicate name", func(t *testing.T) {
		svc := &mockService{
			patchFn: func(_ context.Context, _ uuid.UUID, _ PatchRequest) (*Company, error) {
				return nil, ErrDuplicateName
			},
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodPatch, "/companies/"+id.String(), map[string]any{
			"name": "Taken",
		})
		if w.Code != http.StatusConflict {
			t.Fatalf("expected 409, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc := &mockService{
			patchFn: func(_ context.Context, _ uuid.UUID, _ PatchRequest) (*Company, error) {
				return &Company{ID: id, Name: "Patched", Type: Corporations}, nil
			},
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodPatch, "/companies/"+id.String(), map[string]any{
			"name": "Patched",
		})
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

func TestHandler_Delete(t *testing.T) {
	id := uuid.New()

	t.Run("not found", func(t *testing.T) {
		svc := &mockService{
			deleteFn: func(_ context.Context, _ uuid.UUID) error { return ErrNotFound },
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodDelete, "/companies/"+id.String(), nil)
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", w.Code)
		}
	})

	t.Run("success", func(t *testing.T) {
		svc := &mockService{
			deleteFn: func(_ context.Context, _ uuid.UUID) error { return nil },
		}
		r := newRouter(svc)
		w := doJSON(r, http.MethodDelete, "/companies/"+id.String(), nil)
		if w.Code != http.StatusNoContent {
			t.Fatalf("expected 204, got %d", w.Code)
		}
	})
}
