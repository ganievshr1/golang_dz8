package domain

import (
	"context"
	"time"
)

type BudgetRepository interface {
	Upsert(context.Context, Budget) error
	GetByCategory(context.Context, string) (Budget, error)
	List(context.Context) ([]Budget, error)
}

type ExpenseRepository interface {
	Add(context.Context, Transaction) (Transaction, error)
	List(context.Context) ([]Transaction, error)
	SumByCategory(context.Context, string) (int64, error)
	Summary(context.Context, string, string) ([]ReportItem, error)
}

// Выполняет изменения атомарно и последовательно для указанных категорий.
// Переданные в callback репозитории действуют только внутри этой операции.
type UnitOfWork interface {
	Within(
		context.Context,
		[]string,
		func(BudgetRepository, ExpenseRepository) error,
	) error
}

type ReportCache interface {
	Get(context.Context, string, string) (ReportSummary, bool, error)
	Set(context.Context, ReportSummary, time.Duration) error
}
