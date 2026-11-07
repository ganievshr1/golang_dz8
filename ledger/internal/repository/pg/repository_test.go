package pg

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"example.com/finance-services/ledger/internal/domain"
	"github.com/DATA-DOG/go-sqlmock"
)

func TestRepositoriesAndTransaction(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b, e, u := New(db)
	ctx := context.Background()

	m.ExpectExec("INSERT INTO budgets").
		WithArgs("еда", 1000).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := b.Upsert(ctx, domain.Budget{
		Category: "еда",
		Limit:    1000,
	}); err != nil {
		t.Fatal(err)
	}

	m.ExpectQuery("SELECT .* FROM budgets WHERE").
		WithArgs("еда").
		WillReturnRows(sqlmock.NewRows([]string{"limit"}).AddRow(1000))

	item, err := b.GetByCategory(ctx, "еда")
	if err != nil || item.Limit != 1000 {
		t.Fatal(item, err)
	}

	m.ExpectQuery("SELECT .* FROM budgets WHERE").
		WithArgs("нет").
		WillReturnError(sql.ErrNoRows)

	if _, err := b.GetByCategory(ctx, "нет"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal(err)
	}

	m.ExpectQuery("SELECT category, .* FROM budgets ORDER").
		WillReturnRows(
			sqlmock.NewRows([]string{"category", "limit"}).
				AddRow("еда", 1000),
		)

	if items, err := b.List(ctx); err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}

	m.ExpectQuery("SELECT .*SUM.* FROM expenses WHERE category").
		WithArgs("еда").
		WillReturnRows(sqlmock.NewRows([]string{"sum"}).AddRow(450))

	if sum, err := e.SumByCategory(ctx, "еда"); err != nil || sum != 450 {
		t.Fatal(sum, err)
	}

	m.ExpectBegin()

	m.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("еда").
		WillReturnResult(sqlmock.NewResult(0, 1))

	m.ExpectQuery("INSERT INTO expenses").
		WithArgs(450, "еда", "ланч", "2026-10-30").
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))

	m.ExpectCommit()

	err = u.Within(
		ctx,
		[]string{"еда", "еда"},
		func(_ domain.BudgetRepository, expenses domain.ExpenseRepository) error {
			saved, err := expenses.Add(ctx, domain.Transaction{
				Amount:      450,
				Category:    "еда",
				Description: "ланч",
				Date:        "2026-10-30",
			})

			if err == nil && saved.ID != 42 {
				t.Fatal(saved)
			}
			return err
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	m.ExpectQuery("SELECT id, .* FROM expenses ORDER").
		WillReturnRows(
			sqlmock.NewRows([]string{
				"id", "amount", "category", "description", "date",
			}).AddRow(42, 450, "еда", "ланч", "2026-10-30"),
		)

	if items, err := e.List(ctx); err != nil ||
		len(items) != 1 ||
		items[0].ID != 42 {
		t.Fatal(items, err)
	}

	m.ExpectQuery("SELECT category, .* FROM expenses WHERE date").
		WithArgs("2026-10-01", "2026-10-31").
		WillReturnRows(
			sqlmock.NewRows([]string{"category", "amount"}).
				AddRow("еда", 450),
		)

	if items, err := e.Summary(ctx, "2026-10-01", "2026-10-31"); err != nil ||
		len(items) != 1 ||
		items[0].Amount != 450 {
		t.Fatal(items, err)
	}

	m.ExpectBegin()

	m.ExpectExec("SELECT pg_advisory_xact_lock").
		WithArgs("еда").
		WillReturnResult(sqlmock.NewResult(0, 1))

	m.ExpectRollback()

	err = u.Within(
		ctx,
		[]string{"еда"},
		func(domain.BudgetRepository, domain.ExpenseRepository) error {
			return domain.ErrBudgetExceeded
		},
	)

	if !errors.Is(err, domain.ErrBudgetExceeded) {
		t.Fatal(err)
	}

	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryReadErrors(t *testing.T) {
	db, m, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	b, e, _ := New(db)
	ctx := context.Background()
	cause := errors.New("database down")

	m.ExpectQuery("SELECT").WillReturnError(cause)

	if _, err := b.List(ctx); !errors.Is(err, cause) {
		t.Fatal(err)
	}

	m.ExpectQuery("SELECT").
		WillReturnRows(
			sqlmock.NewRows([]string{
				"id", "amount", "category", "description", "date",
			}).
				AddRow(1, 100, "еда", "", "2026-10-30").
				RowError(0, cause),
		)

	if _, err := e.List(ctx); !errors.Is(err, cause) {
		t.Fatal(err)
	}

	m.ExpectQuery("SELECT").
		WillReturnRows(
			sqlmock.NewRows([]string{"category", "limit"}).
				AddRow("еда", "broken"),
		)

	if _, err := b.List(ctx); err == nil {
		t.Fatal("scan error ignored")
	}

	if err := m.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
