package company

import "github.com/google/uuid"

type Type string

const (
	Corporations       Type = "Corporations"
	NonProfit          Type = "NonProfit"
	Cooperative        Type = "Cooperative"
	SoleProprietorship Type = "Sole Proprietorship"
)

func (t Type) IsValid() bool {
	switch t {
	case Corporations, NonProfit, Cooperative, SoleProprietorship:
		return true
	}
	return false
}

type Company struct {
	ID              uuid.UUID `json:"id"`
	Name            string    `json:"name"`
	Description     string    `json:"description,omitempty"`
	AmountEmployees int       `json:"amount_employees"`
	Registered      bool      `json:"registered"`
	Type            Type      `json:"type"`
}

type CreateRequest struct {
	Name            string `json:"name"             validate:"required,max=15"`
	Description     string `json:"description"      validate:"max=3000"`
	AmountEmployees int    `json:"amount_employees" validate:"required,min=1"`
	Registered      *bool  `json:"registered"       validate:"required"`
	Type            Type   `json:"type"             validate:"required"`
}

// PatchRequest uses pointers so we can tell "not sent" from zero value.
type PatchRequest struct {
	Name            *string `json:"name"             validate:"omitempty,max=15"`
	Description     *string `json:"description"      validate:"omitempty,max=3000"`
	AmountEmployees *int    `json:"amount_employees" validate:"omitempty,min=1"`
	Registered      *bool   `json:"registered"`
	Type            *Type   `json:"type"`
}
