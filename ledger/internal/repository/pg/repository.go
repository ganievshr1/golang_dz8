package pg

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"example.com/finance-services/ledger/internal/domain"
)

type querier interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type Budgets struct {
	q querier
}

type Expenses struct {
	q querier
}

type UnitOfWork struct {
	db *sql.DB
}

func New(db *sql.DB) (*Budgets, *Expenses, *UnitOfWork) {
	return &Budgets{db}, &Expenses{db}, &UnitOfWork{db}
}

var _ domain.BudgetRepository = (*Budgets)(nil)
var _ domain.ExpenseRepository = (*Expenses)(nil)
var _ domain.UnitOfWork = (*UnitOfWork)(nil)

func (u *UnitOfWork) Within(
	ctx context.Context,
	categories []string,
	fn func(domain.BudgetRepository, domain.ExpenseRepository) error,
) error {
	tx, err := u.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelReadCommitted,
	})
	if err != nil {
		return err
	}
	defer tx.Rollback()

	names := append([]string(nil), categories...)
	sort.Strings(names)

	for i, name := range names {
		if i > 0 && name == names[i-1] {
			continue
		}

		_, err := tx.ExecContext(
			ctx,
			"SELECT pg_advisory_xact_lock(hashtextextended($1, 0))",
			name,
		)
		if err != nil {
			return err
		}
	}

	if err := fn(&Budgets{tx}, &Expenses{tx}); err != nil {
		return err
	}

	return tx.Commit()
}

func (r *Budgets) Upsert(ctx context.Context, b domain.Budget) error {
	_, err := r.q.ExecContext(
		ctx,
		"INSERT INTO budgets(category, limit_amount) VALUES($1, $2::numeric/100) ON CONFLICT(category) DO UPDATE SET limit_amount=EXCLUDED.limit_amount",
		b.Category,
		b.Limit,
	)
	return err
}

func (r *Budgets) GetByCategory(
	ctx context.Context,
	category string,
) (domain.Budget, error) {
	b := domain.Budget{Category: category}

	err := r.q.QueryRowContext(
		ctx,
		"SELECT (limit_amount*100)::bigint FROM budgets WHERE category=$1",
		category,
	).Scan(&b.Limit)

	if errors.Is(err, sql.ErrNoRows) {
		return domain.Budget{}, domain.ErrNotFound
	}

	return b, err
}

func (r *Budgets) List(ctx context.Context) ([]domain.Budget, error) {
	rows, err := r.q.QueryContext(
		ctx,
		"SELECT category, (limit_amount*100)::bigint FROM budgets ORDER BY category",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Budget, 0)

	for rows.Next() {
		var b domain.Budget

		if err := rows.Scan(&b.Category, &b.Limit); err != nil {
			return nil, err
		}

		result = append(result, b)
	}

	return result, rows.Err()
}

func (r *Expenses) Add(
	ctx context.Context,
	item domain.Transaction,
) (domain.Transaction, error) {
	err := r.q.QueryRowContext(
		ctx,
		"INSERT INTO expenses(amount, category, description, date) VALUES($1::numeric/100, $2, $3, $4::date) RETURNING id",
		item.Amount,
		item.Category,
		item.Description,
		item.Date,
	).Scan(&item.ID)

	return item, err
}

func (r *Expenses) List(ctx context.Context) ([]domain.Transaction, error) {
	rows, err := r.q.QueryContext(
		ctx,
		"SELECT id, (amount*100)::bigint, category, COALESCE(description, ''), to_char(date, 'YYYY-MM-DD') FROM expenses ORDER BY date DESC, id DESC",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.Transaction, 0)

	for rows.Next() {
		var item domain.Transaction

		if err := rows.Scan(
			&item.ID,
			&item.Amount,
			&item.Category,
			&item.Description,
			&item.Date,
		); err != nil {
			return nil, err
		}

		result = append(result, item)
	}

	return result, rows.Err()
}

func (r *Expenses) SumByCategory(
	ctx context.Context,
	category string,
) (int64, error) {
	var sum int64

	err := r.q.QueryRowContext(
		ctx,
		"SELECT (COALESCE(SUM(amount), 0)*100)::bigint FROM expenses WHERE category=$1",
		category,
	).Scan(&sum)

	return sum, err
}

func (r *Expenses) Summary(
	ctx context.Context,
	from, to string,
) ([]domain.ReportItem, error) {
	rows, err := r.q.QueryContext(
		ctx,
		"SELECT category, (SUM(amount)*100)::bigint FROM expenses WHERE date BETWEEN $1::date AND $2::date GROUP BY category ORDER BY category",
		from,
		to,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make([]domain.ReportItem, 0)

	for rows.Next() {
		var item domain.ReportItem

		if err := rows.Scan(&item.Category, &item.Amount); err != nil {
			return nil, err
		}

		result = append(result, item)
	}

	return result, rows.Err()
}
