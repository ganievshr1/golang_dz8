package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"

	"example.com/finance-services/ledger"
)

const jsonContentType = "application/json; charset=utf-8"
const maxBodyBytes = 1 << 20

type handler struct {
	service ledger.Service
	logger  *log.Logger
}

func NewHandler(s ledger.Service, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}

	return Logging(logger)(&handler{
		service: s,
		logger:  logger,
	})
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case "/api/transactions":
		switch r.Method {
		case http.MethodPost:
			h.createTransaction(w, r)
		case http.MethodGet:
			h.listTransactions(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}

	case "/api/budgets":
		switch r.Method {
		case http.MethodPost:
			h.createBudget(w, r)
		case http.MethodGet:
			h.listBudgets(w, r)
		default:
			methodNotAllowed(w, "GET, POST")
		}

	case "/api/reports/summary":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}

		result, err := h.service.GetReportSummary(
			r.Context(),
			r.URL.Query().Get("from"),
			r.URL.Query().Get("to"),
		)
		if err != nil {
			h.serviceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, result)

	case "/ping":
		if r.Method != http.MethodGet {
			methodNotAllowed(w, "GET")
			return
		}

		writeJSON(
			w,
			http.StatusOK,
			map[string]string{"message": "pong"},
		)

	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *handler) createTransaction(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request CreateTransactionRequest

	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	saved, err := h.service.AddTransaction(
		r.Context(),
		request.ToDomain(),
	)
	if err != nil {
		h.serviceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, transactionResponse(saved))
}

func (h *handler) listTransactions(
	w http.ResponseWriter,
	r *http.Request,
) {
	items, err := h.service.ListTransactions(r.Context())
	if err != nil {
		h.serviceError(w, err)
		return
	}

	response := make([]TransactionResponse, 0, len(items))

	for _, tx := range items {
		response = append(response, transactionResponse(tx))
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *handler) createBudget(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request CreateBudgetRequest

	if err := readJSON(w, r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	b := request.ToDomain()

	if err := h.service.SetBudget(r.Context(), b); err != nil {
		h.serviceError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, budgetResponse(b))
}

func (h *handler) listBudgets(
	w http.ResponseWriter,
	r *http.Request,
) {
	items, err := h.service.ListBudgets(r.Context())
	if err != nil {
		h.serviceError(w, err)
		return
	}

	response := make([]BudgetResponse, 0, len(items))

	for _, b := range items {
		response = append(response, budgetResponse(b))
	}

	writeJSON(w, http.StatusOK, response)
}

func readJSON(
	w http.ResponseWriter,
	r *http.Request,
	destination any,
) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("некорректный JSON: %w", err)
	}

	var extra any

	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New(
			"тело должно содержать ровно один JSON-объект",
		)
	}

	return nil
}

func (h *handler) serviceError(w http.ResponseWriter, err error) {
	var invalid *ledger.ValidationError

	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Error())

	case errors.Is(err, ledger.ErrInvalidPeriod):
		writeError(w, http.StatusBadRequest, err.Error())

	case errors.Is(err, ledger.ErrBudgetExceeded):
		writeError(w, http.StatusConflict, "budget exceeded")

	default:
		h.logger.Printf("internal error: %v", err)
		writeError(
			w,
			http.StatusInternalServerError,
			"internal server error",
		)
	}
}

func methodNotAllowed(w http.ResponseWriter, allow string) {
	w.Header().Set("Allow", allow)
	writeError(w, http.StatusMethodNotAllowed, "method not allowed")
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", jsonContentType)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
