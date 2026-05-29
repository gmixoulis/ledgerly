package company

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound      = errors.New("company not found")
	ErrDuplicateName = errors.New("company name already exists")
)

const pgUniqueViolation = "23505" // unique_violation

type Repository interface {
	Create(ctx context.Context, c *Company) error
	GetByID(ctx context.Context, id uuid.UUID) (*Company, error)
	Update(ctx context.Context, c *Company) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type postgresRepo struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) Repository {
	return &postgresRepo{pool: pool}
}

func (r *postgresRepo) Create(ctx context.Context, c *Company) error {
	const q = `
		INSERT INTO companies (id, name, description, amount_employees, registered, type)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.pool.Exec(ctx, q,
		c.ID, c.Name, c.Description, c.AmountEmployees, c.Registered, string(c.Type),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateName
		}
		return fmt.Errorf("creating company: %w", err)
	}
	return nil
}

func (r *postgresRepo) GetByID(ctx context.Context, id uuid.UUID) (*Company, error) {
	const q = `
		SELECT id, name, COALESCE(description, ''), amount_employees, registered, type
		FROM companies
		WHERE id = $1`

	var c Company
	var compType string
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&c.ID, &c.Name, &c.Description, &c.AmountEmployees, &c.Registered, &compType,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("getting company: %w", err)
	}
	c.Type = Type(compType)
	return &c, nil
}

func (r *postgresRepo) Update(ctx context.Context, c *Company) error {
	const q = `
		UPDATE companies
		SET name = $1, description = $2, amount_employees = $3, registered = $4, type = $5, updated_at = NOW()
		WHERE id = $6`

	tag, err := r.pool.Exec(ctx, q,
		c.Name, c.Description, c.AmountEmployees, c.Registered, string(c.Type), c.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrDuplicateName
		}
		return fmt.Errorf("updating company: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *postgresRepo) Delete(ctx context.Context, id uuid.UUID) error {
	const q = `DELETE FROM companies WHERE id = $1`

	tag, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("deleting company: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation
}
