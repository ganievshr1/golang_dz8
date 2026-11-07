package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const maxMoney int64 = 99_999_999_999_999

var (
	ErrBudgetExceeded = errors.New("budget exceeded")
	ErrNotFound       = errors.New("not found")
	ErrInvalidPeriod  = errors.New("некорректный период отчёта")
)

type ValidationError struct {
	Err error
}

func (e *ValidationError) Error() string {
	return e.Err.Error()
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

type Transaction struct {
	ID          int
	Amount      int // Копейки.
	Category    string
	Description string
	Date        string
}

func (tx Transaction) Validate() error {
	if tx.Amount <= 0 {
		return errors.New("сумма транзакции должна быть положительной")
	}

	if int64(tx.Amount) > maxMoney {
		return errors.New("сумма превышает диапазон NUMERIC(14,2)")
	}

	if strings.TrimSpace(tx.Category) == "" {
		return errors.New("категория транзакции не должна быть пустой")
	}

	date, err := time.Parse("2006-01-02", tx.Date)
	if err != nil {
		return fmt.Errorf("дата должна быть в формате YYYY-MM-DD: %w", err)
	}

	if date.Year() < 1 {
		return errors.New("год даты должен быть не меньше 0001")
	}

	return nil
}

type Budget struct {
	Category string `json:"category"`
	Limit    int    `json:"limit"` // Копейки.
}

func (b Budget) Validate() error {
	if b.Limit <= 0 {
		return errors.New("лимит бюджета должен быть положительным")
	}

	if int64(b.Limit) > maxMoney {
		return errors.New("лимит превышает диапазон NUMERIC(14,2)")
	}

	if strings.TrimSpace(b.Category) == "" {
		return errors.New("категория бюджета не должна быть пустой")
	}

	return nil
}

type Validatable interface {
	Validate() error
}

var _ Validatable = Transaction{}
var _ Validatable = Budget{}

func CheckValid(v Validatable) error {
	if v == nil {
		return errors.New("объект для проверки не задан")
	}

	return v.Validate()
}

type ReportItem struct {
	Category string `json:"category"`
	Amount   int64  `json:"amount"`
}

type ReportSummary struct {
	From  string       `json:"from"`
	To    string       `json:"to"`
	Total int64        `json:"total"`
	Items []ReportItem `json:"items"`
}
