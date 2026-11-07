package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"time"

	"example.com/finance-services/ledger/internal/domain"
)

const operationTimeout = 5 * time.Second
const reportTTL = 30 * time.Second

type Service interface {
	SetBudget(context.Context, domain.Budget) error
	ListBudgets(context.Context) ([]domain.Budget, error)

	AddTransaction(
		context.Context,
		domain.Transaction,
	) (domain.Transaction, error)

	ListTransactions(context.Context) ([]domain.Transaction, error)

	GetReportSummary(
		context.Context,
		string,
		string,
	) (domain.ReportSummary, error)

	LoadBudgets(context.Context, io.Reader) error
}

type ledgerService struct {
	budgets  domain.BudgetRepository
	expenses domain.ExpenseRepository
	work     domain.UnitOfWork
	cache    domain.ReportCache
}

func New(
	b domain.BudgetRepository,
	e domain.ExpenseRepository,
	u domain.UnitOfWork,
	c domain.ReportCache,
) Service {
	return &ledgerService{
		budgets:  b,
		expenses: e,
		work:     u,
		cache:    c,
	}
}

func (s *ledgerService) AddTransaction(
	ctx context.Context,
	item domain.Transaction,
) (domain.Transaction, error) {
	if err := item.Validate(); err != nil {
		return domain.Transaction{}, &domain.ValidationError{Err: err}
	}

	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	var saved domain.Transaction

	err := s.work.Within(
		ctx,
		[]string{item.Category},
		func(b domain.BudgetRepository, e domain.ExpenseRepository) error {
			budget, err := b.GetByCategory(ctx, item.Category)

			if err != nil && !errors.Is(err, domain.ErrNotFound) {
				return err
			}

			if err == nil {
				spent, err := e.SumByCategory(ctx, item.Category)
				if err != nil {
					return err
				}

				// Вычитание безопасно: оба поля ограничены в Validate.
				if spent > int64(budget.Limit)-int64(item.Amount) {
					return domain.ErrBudgetExceeded
				}
			}

			saved, err = e.Add(ctx, item)
			return err
		},
	)

	if err != nil {
		return domain.Transaction{}, err
	}

	return saved, nil
}

func (s *ledgerService) SetBudget(
	ctx context.Context,
	b domain.Budget,
) error {
	return s.setBudgets(ctx, []domain.Budget{b})
}

func (s *ledgerService) setBudgets(
	ctx context.Context,
	items []domain.Budget,
) error {
	names := make([]string, 0, len(items))

	for _, b := range items {
		if err := b.Validate(); err != nil {
			return &domain.ValidationError{Err: err}
		}

		names = append(names, b.Category)
	}

	if len(items) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	return s.work.Within(
		ctx,
		names,
		func(b domain.BudgetRepository, _ domain.ExpenseRepository) error {
			for _, item := range items {
				if err := b.Upsert(ctx, item); err != nil {
					return err
				}
			}

			return nil
		},
	)
}

func (s *ledgerService) ListBudgets(
	ctx context.Context,
) ([]domain.Budget, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	return s.budgets.List(ctx)
}

func (s *ledgerService) ListTransactions(
	ctx context.Context,
) ([]domain.Transaction, error) {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	return s.expenses.List(ctx)
}

func (s *ledgerService) LoadBudgets(
	ctx context.Context,
	r io.Reader,
) error {
	if r == nil {
		return errors.New("загрузка бюджетов: источник чтения не задан")
	}

	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()

	var items []domain.Budget

	if err := decoder.Decode(&items); err != nil {
		return fmt.Errorf("загрузка бюджетов: чтение JSON: %w", err)
	}

	if items == nil {
		return errors.New(
			"загрузка бюджетов: ожидается JSON-массив, а не null",
		)
	}

	var extra any

	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("загрузка бюджетов: лишние данные после массива")
	}

	return s.setBudgets(ctx, items)
}

func (s *ledgerService) GetReportSummary(
	ctx context.Context,
	from, to string,
) (domain.ReportSummary, error) {
	start, e1 := time.Parse("2006-01-02", from)
	end, e2 := time.Parse("2006-01-02", to)

	if e1 != nil ||
		e2 != nil ||
		start.Year() < 1 ||
		end.Year() < 1 ||
		start.After(end) {
		return domain.ReportSummary{}, fmt.Errorf(
			"%w: нужны from <= to в формате YYYY-MM-DD",
			domain.ErrInvalidPeriod,
		)
	}

	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return domain.ReportSummary{}, err
	}

	if s.cache != nil {
		result, hit, err := s.cache.Get(ctx, from, to)

		if err == nil && hit {
			log.Printf("report cache hit: %s:%s", from, to)
			return result, nil
		}

		if err != nil {
			log.Printf("report cache read: %v", err)
		}
	}

	log.Printf("report cache miss: %s:%s", from, to)

	items, err := s.expenses.Summary(ctx, from, to)
	if err != nil {
		return domain.ReportSummary{}, err
	}

	if items == nil {
		items = []domain.ReportItem{}
	}

	result := domain.ReportSummary{
		From:  from,
		To:    to,
		Items: items,
	}

	for _, item := range items {
		if (item.Amount > 0 &&
			result.Total > math.MaxInt64-item.Amount) ||
			(item.Amount < 0 &&
				result.Total < math.MinInt64-item.Amount) {
			return domain.ReportSummary{}, errors.New(
				"сумма отчёта выходит за диапазон int64",
			)
		}

		result.Total += item.Amount
	}

	if s.cache != nil {
		if err := s.cache.Set(ctx, result, reportTTL); err != nil {
			log.Printf("report cache write: %v", err)
		}
	}

	return result, nil
}
