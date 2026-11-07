// Package ledger предоставляет публичный интерфейс приложения.
package ledger

import (
	"context"

	"example.com/finance-services/ledger/internal/app"
	"example.com/finance-services/ledger/internal/domain"
	"example.com/finance-services/ledger/internal/service"
)

type Service = service.Service

type Transaction = domain.Transaction
type Budget = domain.Budget

type ReportItem = domain.ReportItem
type ReportSummary = domain.ReportSummary

type ValidationError = domain.ValidationError

var ErrBudgetExceeded = domain.ErrBudgetExceeded
var ErrInvalidPeriod = domain.ErrInvalidPeriod

func New(ctx context.Context) (Service, func() error, error) {
	return app.New(ctx)
}
