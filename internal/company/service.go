package company

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

type EventPublisher interface {
	Publish(ctx context.Context, eventType string, payload any) error
}

type Service interface {
	Create(ctx context.Context, req CreateRequest) (*Company, error)
	GetByID(ctx context.Context, id uuid.UUID) (*Company, error)
	Patch(ctx context.Context, id uuid.UUID, req PatchRequest) (*Company, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type service struct {
	repo      Repository
	publisher EventPublisher
}

func NewService(repo Repository, publisher EventPublisher) Service {
	return &service{repo: repo, publisher: publisher}
}

const publishTimeout = 5 * time.Second

// publish uses its own context so a cancelled request ctx doesn't drop the event.
func (s *service) publish(eventType string, payload any) {
	ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
	defer cancel()
	if err := s.publisher.Publish(ctx, eventType, payload); err != nil {
		log.Printf("warn: publish %q failed: %v", eventType, err)
	}
}

func (s *service) Create(ctx context.Context, req CreateRequest) (*Company, error) {
	if !req.Type.IsValid() {
		return nil, fmt.Errorf("invalid company type: %q", req.Type)
	}

	c := &Company{
		ID:              uuid.New(),
		Name:            req.Name,
		Description:     req.Description,
		AmountEmployees: req.AmountEmployees,
		Registered:      *req.Registered,
		Type:            req.Type,
	}

	if err := s.repo.Create(ctx, c); err != nil {
		return nil, err
	}

	s.publish("company.created", c)
	return c, nil
}

func (s *service) GetByID(ctx context.Context, id uuid.UUID) (*Company, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *service) Patch(ctx context.Context, id uuid.UUID, req PatchRequest) (*Company, error) {
	c, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if req.Name != nil {
		c.Name = *req.Name
	}
	if req.Description != nil {
		c.Description = *req.Description
	}
	if req.AmountEmployees != nil {
		c.AmountEmployees = *req.AmountEmployees
	}
	if req.Registered != nil {
		c.Registered = *req.Registered
	}
	if req.Type != nil {
		if !req.Type.IsValid() {
			return nil, fmt.Errorf("invalid company type: %q", *req.Type)
		}
		c.Type = *req.Type
	}

	if err := s.repo.Update(ctx, c); err != nil {
		return nil, err
	}

	s.publish("company.updated", c)
	return c, nil
}

func (s *service) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}
	s.publish("company.deleted", map[string]string{"id": id.String()})
	return nil
}
