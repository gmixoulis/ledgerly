package company

import "testing"

func TestType_IsValid(t *testing.T) {
	tests := []struct {
		name string
		typ  Type
		want bool
	}{
		{"Corporations valid", Corporations, true},
		{"NonProfit valid", NonProfit, true},
		{"Cooperative valid", Cooperative, true},
		{"SoleProprietorship valid", SoleProprietorship, true},
		{"empty invalid", Type(""), false},
		{"random string invalid", Type("LLC"), false},
		{"wrong casing invalid", Type("corporations"), false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.typ.IsValid(); got != tc.want {
				t.Errorf("Type(%q).IsValid() = %v, want %v", tc.typ, got, tc.want)
			}
		})
	}
}
