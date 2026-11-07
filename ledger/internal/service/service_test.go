package service

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"example.com/finance-services/ledger/internal/domain"
)

type budgetsFake struct {
	data map[string]domain.Budget
	err  error
}

func (b *budgetsFake) Upsert(
	_ context.Context,
	item domain.Budget,
) error {
	if b.err != nil {
		return b.err
	}
	b.data[item.Category] = item
	return nil
}

func (b *budgetsFake) GetByCategory(
	_ context.Context,
	name string,
) (domain.Budget, error) {
	if b.err != nil {
		return domain.Budget{}, b.err
	}

	v, ok := b.data[name]
	if !ok {
		return v, domain.ErrNotFound
	}
	return v, nil
}

func (b *budgetsFake) List(context.Context) ([]domain.Budget, error) {
	result := []domain.Budget{}
	for _, v := range b.data {
		result = append(result, v)
	}
	return result, b.err
}

type expensesFake struct {
	data    []domain.Transaction
	err     error
	sums    []domain.ReportItem
	queries int
}

func (e *expensesFake) Add(
	_ context.Context,
	v domain.Transaction,
) (domain.Transaction, error) {
	if e.err != nil {
		return domain.Transaction{}, e.err
	}

	v.ID = len(e.data) + 1
	e.data = append(e.data, v)
	return v, nil
}

func (e *expensesFake) List(context.Context) ([]domain.Transaction, error) {
	return e.data, e.err
}

func (e *expensesFake) SumByCategory(
	_ context.Context,
	name string,
) (int64, error) {
	var sum int64
	for _, v := range e.data {
		if v.Category == name {
			sum += int64(v.Amount)
		}
	}
	return sum, e.err
}

func (e *expensesFake) Summary(
	context.Context,
	string,
	string,
) ([]domain.ReportItem, error) {
	e.queries++
	return e.sums, e.err
}

type workFake struct {
	b     *budgetsFake
	e     *expensesFake
	calls int
}

func (w *workFake) Within(
	ctx context.Context,
	_ []string,
	fn func(domain.BudgetRepository, domain.ExpenseRepository) error,
) error {
	w.calls++
	if err := ctx.Err(); err != nil {
		return err
	}
	return fn(w.b, w.e)
}

func setup() (Service, *budgetsFake, *expensesFake, *workFake) {
	b := &budgetsFake{data: map[string]domain.Budget{}}
	e := &expensesFake{data: []domain.Transaction{}}
	w := &workFake{b: b, e: e}
	return New(b, e, w, nil), b, e, w
}

func TestBudgetRule(t *testing.T) {
	tests := []struct {
		name                 string
		limit, spent, amount int
		exceeded             bool
	}{
		{"below", 1000, 100, 450, false},
		{"exact", 1000, 550, 450, false},
		{"single", 1000, 0, 1001, true},
		{"sum", 1000, 600, 450, true},
		{"no_budget", 0, 100, 5000, false},
		{"large_spent", 1000, math.MaxInt64, 1, true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, b, e, _ := setup()

			if tc.limit > 0 {
				b.data["еда"] = domain.Budget{
					Category: "еда",
					Limit:    tc.limit,
				}
			}

			e.data = append(e.data, domain.Transaction{
				ID:       1,
				Category: "еда",
				Amount:   tc.spent,
			})

			saved, err := s.AddTransaction(
				context.Background(),
				domain.Transaction{
					Amount:   tc.amount,
					Category: "еда",
					Date:     "2026-10-30",
				},
			)

			if tc.exceeded {
				if !errors.Is(err, domain.ErrBudgetExceeded) ||
					len(e.data) != 1 ||
					saved.ID != 0 {
					t.Fatalf("saved=%+v err=%v", saved, err)
				}
			} else if err != nil || saved.ID != 2 || len(e.data) != 2 {
				t.Fatalf("saved=%+v err=%v", saved, err)
			}
		})
	}
}

func TestValidationAndRepositoryErrors(t *testing.T) {
	s, b, e, w := setup()
	ctx := context.Background()

	_, err := s.AddTransaction(ctx, domain.Transaction{})
	var invalid *domain.ValidationError

	if !errors.As(err, &invalid) {
		t.Fatal(err)
	}

	if err := s.SetBudget(ctx, domain.Budget{}); !errors.As(err, &invalid) {
		t.Fatal(err)
	}

	if w.calls != 0 {
		t.Fatal("invalid data reached repositories")
	}

	for _, limit := range []int{1000, 2000} {
		if err := s.SetBudget(ctx, domain.Budget{
			Category: "еда",
			Limit:    limit,
		}); err != nil {
			t.Fatal(err)
		}
	}

	list, err := s.ListBudgets(ctx)
	if err != nil || len(list) != 1 || list[0].Limit != 2000 {
		t.Fatal(list, err)
	}

	cause := errors.New("storage unavailable")
	b.err = cause

	if _, err := s.AddTransaction(ctx, domain.Transaction{
		Amount:   1,
		Category: "еда",
		Date:     "2026-10-30",
	}); !errors.Is(err, cause) {
		t.Fatal(err)
	}

	b.err = nil
	e.err = cause

	if _, err := s.AddTransaction(ctx, domain.Transaction{
		Amount:   1,
		Category: "еда",
		Date:     "2026-10-30",
	}); !errors.Is(err, cause) {
		t.Fatal(err)
	}

	if len(e.data) != 0 {
		t.Fatal("failed write saved")
	}
}

func TestLoadBudgets(t *testing.T) {
	s, b, _, w := setup()
	ctx := context.Background()

	inputs := []string{
		"",
		"null",
		"{}",
		"[",
		"[null]",
		`[{}]`,
		`[{"category":"еда","limit":0}]`,
		`[{"category":"еда","limit":1.5}]`,
		`[{"category":"еда","limit":100,"extra":1}]`,
		`[] {}`,
		`[] broken`,
		`[{"category":"еда","limit":100},{"category":"досуг","limit":-1}]`,
	}

	for _, input := range inputs {
		if err := s.LoadBudgets(ctx, strings.NewReader(input)); err == nil {
			t.Fatal("accepted", input)
		}
	}

	if s.LoadBudgets(ctx, nil) == nil || w.calls != 0 {
		t.Fatal("invalid batch reached storage")
	}

	err := s.LoadBudgets(
		ctx,
		strings.NewReader(
			`[{"category":"еда","limit":100},{"category":"еда","limit":200}]`,
		),
	)
	if err != nil {
		t.Fatal(err)
	}

	if len(b.data) != 1 || b.data["еда"].Limit != 200 {
		t.Fatal(b.data)
	}
}

type cacheFake struct {
	value domain.ReportSummary
	hit   bool
	err   error
	ttl   time.Duration
}

func (c *cacheFake) Get(
	context.Context,
	string,
	string,
) (domain.ReportSummary, bool, error) {
	return c.value, c.hit, c.err
}

func (c *cacheFake) Set(
	_ context.Context,
	v domain.ReportSummary,
	ttl time.Duration,
) error {
	c.value = v
	c.hit = true
	c.ttl = ttl
	return c.err
}

func TestReport(t *testing.T) {
	_, b, e, w := setup()
	c := &cacheFake{}
	s := New(b, e, w, c)
	ctx := context.Background()

	e.sums = []domain.ReportItem{
		{Category: "еда", Amount: 450},
		{Category: "досуг", Amount: 550},
	}

	first, err := s.GetReportSummary(ctx, "2026-10-01", "2026-10-31")
	if err != nil || first.Total != 1000 || c.ttl != 30*time.Second {
		t.Fatal(first, err, c.ttl)
	}

	e.err = errors.New("DB down")

	hit, err := s.GetReportSummary(ctx, first.From, first.To)
	if err != nil || hit.Total != 1000 || e.queries != 1 {
		t.Fatal(hit, err)
	}

	e.err = nil
	c.err = errors.New("Redis down")
	e.sums = nil

	fresh, err := s.GetReportSummary(ctx, first.From, first.To)
	if err != nil ||
		fresh.Total != 0 ||
		fresh.Items == nil ||
		e.queries != 2 {
		t.Fatal(fresh, err)
	}

	if _, err := s.GetReportSummary(
		ctx, "bad", first.To,
	); !errors.Is(err, domain.ErrInvalidPeriod) {
		t.Fatal(err)
	}

	if _, err := s.GetReportSummary(
		ctx, first.To, first.From,
	); !errors.Is(err, domain.ErrInvalidPeriod) {
		t.Fatal(err)
	}

	canceled, cancel := context.WithCancel(ctx)
	cancel()

	if _, err := s.GetReportSummary(
		canceled, first.From, first.To,
	); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	e.sums = []domain.ReportItem{
		{Amount: math.MaxInt64},
		{Amount: 1},
	}

	if _, err := s.GetReportSummary(ctx, first.From, first.To); err == nil {
		t.Fatal("overflow accepted")
	}
}
