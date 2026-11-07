package domain

import (
	"errors"
	"testing"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		v       Validatable
		invalid bool
	}{
		{"transaction", Transaction{Amount: 100, Category: "еда", Date: "2026-10-30"}, false},
		{"zero", Transaction{Amount: 0, Category: "еда", Date: "2026-10-30"}, true},
		{"negative", Transaction{Amount: -1, Category: "еда", Date: "2026-10-30"}, true},
		{"large", Transaction{Amount: int(maxMoney + 1), Category: "еда", Date: "2026-10-30"}, true},
		{"category", Transaction{Amount: 1, Category: " \t", Date: "2026-10-30"}, true},
		{"missing_date", Transaction{Amount: 1, Category: "еда"}, true},
		{"date_format", Transaction{Amount: 1, Category: "еда", Date: "30.10.2026"}, true},
		{"date", Transaction{Amount: 1, Category: "еда", Date: "2026-02-30"}, true},
		{"zero_year", Transaction{Amount: 1, Category: "еда", Date: "0000-01-01"}, true},
		{"leap", Transaction{Amount: 1, Category: "еда", Date: "2024-02-29"}, false},
		{"not_leap", Transaction{Amount: 1, Category: "еда", Date: "2025-02-29"}, true},
		{"budget", Budget{Category: "еда", Limit: 100}, false},
		{"zero_limit", Budget{Category: "еда"}, true},
		{"negative_limit", Budget{Category: "еда", Limit: -1}, true},
		{"large_limit", Budget{Category: "еда", Limit: int(maxMoney + 1)}, true},
		{"budget_category", Budget{Category: " ", Limit: 1}, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := CheckValid(tc.v); (err != nil) != tc.invalid {
				t.Fatalf("error=%v", err)
			}
		})
	}

	if CheckValid(nil) == nil {
		t.Fatal("nil accepted")
	}

	p := &probe{err: errors.New("invalid")}

	if CheckValid(p) != p.err || !p.called {
		t.Fatal("Validate not delegated")
	}
}

type probe struct {
	called bool
	err    error
}

func (p *probe) Validate() error {
	p.called = true
	return p.err
}
