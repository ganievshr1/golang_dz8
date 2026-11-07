package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/finance-services/ledger"
)

// Заглушка интерфейса сервиса.
// Реальная бизнес-логика тестируется в Ledger.
type fakeService struct {
	err          error
	saved        ledger.Transaction
	budget       ledger.Budget
	transactions []ledger.Transaction
	budgets      []ledger.Budget
	writes       int
}

var _ ledger.Service = (*fakeService)(nil)

func (s *fakeService) AddTransaction(
	_ context.Context,
	tx ledger.Transaction,
) (ledger.Transaction, error) {
	if err := tx.Validate(); err != nil {
		return ledger.Transaction{}, &ledger.ValidationError{Err: err}
	}

	if s.err != nil {
		return ledger.Transaction{}, s.err
	}

	s.writes++
	tx.ID = 42
	s.saved = tx
	return tx, nil
}

func (s *fakeService) SetBudget(
	_ context.Context,
	b ledger.Budget,
) error {
	if err := b.Validate(); err != nil {
		return &ledger.ValidationError{Err: err}
	}

	if s.err != nil {
		return s.err
	}

	s.writes++
	s.budget = b
	return nil
}

func (s *fakeService) ListTransactions(
	context.Context,
) ([]ledger.Transaction, error) {
	return s.transactions, s.err
}

func (s *fakeService) ListBudgets(
	context.Context,
) ([]ledger.Budget, error) {
	return s.budgets, s.err
}

func (s *fakeService) LoadBudgets(context.Context, io.Reader) error {
	return s.err
}

func (s *fakeService) GetReportSummary(
	context.Context,
	string,
	string,
) (ledger.ReportSummary, error) {
	return ledger.ReportSummary{
		From:  "2026-10-01",
		To:    "2026-10-31",
		Total: 450,
		Items: []ledger.ReportItem{
			{Category: "еда", Amount: 450},
		},
	}, s.err
}

func request(
	t *testing.T,
	h http.Handler,
	method, path, body string,
	status int,
) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	h.ServeHTTP(
		w,
		httptest.NewRequest(method, path, strings.NewReader(body)),
	)

	if w.Code != status {
		t.Fatalf(
			"%s %s: status=%d body=%s",
			method, path, w.Code, w.Body,
		)
	}

	if w.Header().Get("Content-Type") != jsonContentType ||
		!json.Valid(w.Body.Bytes()) {
		t.Fatal("invalid JSON/header")
	}

	if status >= 400 {
		var result map[string]string

		if json.Unmarshal(w.Body.Bytes(), &result) != nil ||
			len(result) != 1 ||
			result["error"] == "" {
			t.Fatal("invalid error", w.Body)
		}
	}

	return w
}

func TestHTTPContracts(t *testing.T) {
	s := &fakeService{}
	var logs bytes.Buffer
	h := NewHandler(s, log.New(&logs, "", 0))

	for _, path := range []string{
		"/api/transactions",
		"/api/budgets",
	} {
		w := request(t, h, "GET", path, "", 200)

		if strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatal("want []")
		}
	}

	w := request(
		t, h, "POST", "/api/transactions",
		`{"amount":450,"category":"еда","description":"ланч","date":"2026-10-30"}`,
		201,
	)

	var saved TransactionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}

	want := TransactionResponse{
		ID:          42,
		Amount:      450,
		Category:    "еда",
		Description: "ланч",
		Date:        "2026-10-30",
	}

	if saved != want {
		t.Fatal(saved)
	}

	w = request(
		t, h, "POST", "/api/budgets",
		`{"category":"еда","limit":1000}`,
		201,
	)

	var budget BudgetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &budget); err != nil ||
		budget.Category != "еда" ||
		budget.Limit != 1000 {
		t.Fatal(budget, err)
	}

	s.transactions = []ledger.Transaction{s.saved}
	s.budgets = []ledger.Budget{s.budget}

	w = request(t, h, "GET", "/api/transactions", "", 200)

	var txs []TransactionResponse
	if err := json.Unmarshal(w.Body.Bytes(), &txs); err != nil ||
		len(txs) != 1 ||
		txs[0] != saved {
		t.Fatal("transactions list", err)
	}

	w = request(t, h, "GET", "/api/budgets", "", 200)

	var budgets []BudgetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &budgets); err != nil ||
		len(budgets) != 1 ||
		budgets[0] != budget {
		t.Fatal("budgets list", err)
	}

	w = request(
		t, h, "GET",
		"/api/reports/summary?from=2026-10-01&to=2026-10-31",
		"", 200,
	)

	var report ledger.ReportSummary
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil ||
		report.Total != 450 {
		t.Fatal(report, err)
	}

	request(t, h, "GET", "/missing", "", 404)
	request(t, h, "DELETE", "/api/budgets", "", 405)
	request(t, h, "POST", "/api/reports/summary", "", 405)

	for _, fragment := range []string{
		"method=GET",
		"method=POST",
		`path="/api/transactions"`,
		"duration=",
	} {
		if !strings.Contains(logs.String(), fragment) {
			t.Fatal("missing log", fragment)
		}
	}
}

func TestInvalidHTTP(t *testing.T) {
	s := &fakeService{}
	h := NewHandler(s, nil)

	for _, path := range []string{
		"/api/transactions",
		"/api/budgets",
	} {
		for _, body := range []string{
			"",
			"null",
			"[]",
			"{",
			"{}",
			`{"unknown":1}`,
			"{} {}",
			"{} broken",
			strings.Repeat(" ", maxBodyBytes+1),
		} {
			request(t, h, "POST", path, body, 400)
		}
	}

	for _, body := range []string{
		`{"amount":0,"category":"еда","date":"2026-10-30"}`,
		`{"amount":-1,"category":"еда","date":"2026-10-30"}`,
		`{"amount":1.5,"category":"еда","date":"2026-10-30"}`,
		`{"amount":1,"category":" ","date":"2026-10-30"}`,
		`{"amount":1,"category":"еда","date":"2026-02-30"}`,
	} {
		request(t, h, "POST", "/api/transactions", body, 400)
	}

	for _, body := range []string{
		`{"category":"еда","limit":0}`,
		`{"category":"еда","limit":-1}`,
		`{"category":" ","limit":100}`,
	} {
		request(t, h, "POST", "/api/budgets", body, 400)
	}

	if s.writes != 0 {
		t.Fatal("invalid request saved")
	}
}

func TestHTTPErrors(t *testing.T) {
	s := &fakeService{err: errors.New("private SQL error")}
	var logs bytes.Buffer
	h := NewHandler(s, log.New(&logs, "", 0))

	tests := []struct {
		method, path, body string
	}{
		{"GET", "/api/transactions", ""},
		{"GET", "/api/budgets", ""},
		{"GET", "/api/reports/summary?from=2026-10-01&to=2026-10-31", ""},
		{"POST", "/api/transactions", `{"amount":1,"category":"еда","date":"2026-10-30"}`},
		{"POST", "/api/budgets", `{"category":"еда","limit":100}`},
	}

	for _, tc := range tests {
		w := request(t, h, tc.method, tc.path, tc.body, 500)

		if strings.TrimSpace(w.Body.String()) !=
			`{"error":"internal server error"}` {
			t.Fatal("private error exposed")
		}
	}

	if !strings.Contains(logs.String(), "private SQL error") {
		t.Fatal("error not logged")
	}

	s.err = fmt.Errorf("wrapped: %w", ledger.ErrBudgetExceeded)

	w := request(
		t, h, "POST", "/api/transactions",
		`{"amount":1,"category":"еда","date":"2026-10-30"}`,
		409,
	)

	if strings.TrimSpace(w.Body.String()) !=
		`{"error":"budget exceeded"}` {
		t.Fatal(w.Body)
	}

	s.err = ledger.ErrInvalidPeriod

	request(
		t, h, "GET",
		"/api/reports/summary?from=bad&to=bad",
		"", 400,
	)

	s.err = &ledger.ValidationError{
		Err: errors.New("service validation"),
	}

	request(
		t, h, "POST", "/api/budgets",
		`{"category":"еда","limit":100}`,
		400,
	)
}
