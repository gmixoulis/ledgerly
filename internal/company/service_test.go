package company

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/gmixoulis/ledgerly/internal/events"
)

// fakeRepo is an in-memory Repository implementation for unit tests.
type fakeRepo struct {
	mu   sync.Mutex
	data map[uuid.UUID]*Company
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{data: make(map[uuid.UUID]*Company)}
}

func (r *fakeRepo) Create(_ context.Context, c *Company) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.data {
		if existing.Name == c.Name {
			return ErrDuplicateName
		}
	}
	cp := *c
	r.data[c.ID] = &cp
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*Company, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.data[id]
	if !ok {
		return nil, ErrNotFound
	}
	cp := *c
	return &cp, nil
}

func (r *fakeRepo) Update(_ context.Context, c *Company) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[c.ID]; !ok {
		return ErrNotFound
	}
	cp := *c
	r.data[c.ID] = &cp
	return nil
}

func (r *fakeRepo) Delete(_ context.Context, id uuid.UUID) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[id]; !ok {
		return ErrNotFound
	}
	delete(r.data, id)
	return nil
}

func newSvc() (Service, *fakeRepo) {
	repo := newFakeRepo()
	return NewService(repo, events.NopProducer{}), repo
}

func boolPtr(b bool) *bool { return &b }

func TestService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc, _ := newSvc()
		c, err := svc.Create(context.Background(), CreateRequest{
			Name:            "Acme",
			Description:     "desc",
			AmountEmployees: 10,
			Registered:      boolPtr(true),
			Type:            Corporations,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if c.ID == uuid.Nil {
			t.Fatal("expected non-nil ID")
		}
		if c.Name != "Acme" || c.AmountEmployees != 10 || !c.Registered || c.Type != Corporations {
			t.Fatalf("unexpected company: %+v", c)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		svc, _ := newSvc()
		_, err := svc.Create(context.Background(), CreateRequest{
			Name:            "Bad",
			AmountEmployees: 1,
			Registered:      boolPtr(false),
			Type:            Type("LLC"),
		})
		if err == nil {
			t.Fatal("expected error for invalid type")
		}
	})
}

func TestService_GetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc, _ := newSvc()
		created, _ := svc.Create(context.Background(), CreateRequest{
			Name: "Get", AmountEmployees: 1, Registered: boolPtr(false), Type: NonProfit,
		})
		got, err := svc.GetByID(context.Background(), created.ID)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != created.ID {
			t.Fatalf("id mismatch: got %s want %s", got.ID, created.ID)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc, _ := newSvc()
		_, err := svc.GetByID(context.Background(), uuid.New())
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestService_Patch(t *testing.T) {
	ctx := context.Background()

	mkPatchTest := func(req func() PatchRequest, check func(t *testing.T, c *Company)) func(*testing.T) {
		return func(t *testing.T) {
			svc, _ := newSvc()
			created, _ := svc.Create(ctx, CreateRequest{
				Name: "Orig", Description: "olddesc", AmountEmployees: 5,
				Registered: boolPtr(false), Type: Corporations,
			})
			patched, err := svc.Patch(ctx, created.ID, req())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			check(t, patched)
		}
	}

	t.Run("patch name", mkPatchTest(
		func() PatchRequest { s := "NewName"; return PatchRequest{Name: &s} },
		func(t *testing.T, c *Company) {
			if c.Name != "NewName" {
				t.Fatalf("got name %q", c.Name)
			}
		},
	))
	t.Run("patch description", mkPatchTest(
		func() PatchRequest { s := "newdesc"; return PatchRequest{Description: &s} },
		func(t *testing.T, c *Company) {
			if c.Description != "newdesc" {
				t.Fatalf("got desc %q", c.Description)
			}
		},
	))
	t.Run("patch amount_employees", mkPatchTest(
		func() PatchRequest { n := 99; return PatchRequest{AmountEmployees: &n} },
		func(t *testing.T, c *Company) {
			if c.AmountEmployees != 99 {
				t.Fatalf("got %d", c.AmountEmployees)
			}
		},
	))
	t.Run("patch registered", mkPatchTest(
		func() PatchRequest { b := true; return PatchRequest{Registered: &b} },
		func(t *testing.T, c *Company) {
			if !c.Registered {
				t.Fatal("expected registered true")
			}
		},
	))
	t.Run("patch type", mkPatchTest(
		func() PatchRequest { typ := NonProfit; return PatchRequest{Type: &typ} },
		func(t *testing.T, c *Company) {
			if c.Type != NonProfit {
				t.Fatalf("got type %q", c.Type)
			}
		},
	))

	t.Run("not found", func(t *testing.T) {
		svc, _ := newSvc()
		s := "X"
		_, err := svc.Patch(ctx, uuid.New(), PatchRequest{Name: &s})
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("invalid type", func(t *testing.T) {
		svc, _ := newSvc()
		created, _ := svc.Create(ctx, CreateRequest{
			Name: "P", AmountEmployees: 1, Registered: boolPtr(false), Type: Corporations,
		})
		bad := Type("LLC")
		_, err := svc.Patch(ctx, created.ID, PatchRequest{Type: &bad})
		if err == nil {
			t.Fatal("expected error for invalid type")
		}
	})
}

func TestService_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, _ := newSvc()
		created, _ := svc.Create(ctx, CreateRequest{
			Name: "Del", AmountEmployees: 1, Registered: boolPtr(false), Type: Cooperative,
		})
		if err := svc.Delete(ctx, created.ID); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := svc.GetByID(ctx, created.ID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound after delete, got %v", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svc, _ := newSvc()
		if err := svc.Delete(ctx, uuid.New()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}
