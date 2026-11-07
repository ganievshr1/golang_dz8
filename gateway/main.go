package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"example.com/finance-services/gateway/internal/api"
	"example.com/finance-services/ledger"
)

func newHandler(svc ledger.Service) http.Handler {
	return api.NewHandler(svc, log.Default())
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
	)
	defer stop()

	svc, closeFn, err := ledger.New(ctx)
	if err != nil {
		return err
	}
	defer closeFn()

	server := &http.Server{
		Addr:              ":8080",
		Handler:           newHandler(svc),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	result := make(chan error, 1)

	go func() {
		result <- server.ListenAndServe()
	}()

	log.Print(
		"Gateway service started on http://localhost:8080",
	)

	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err

	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(),
			10*time.Second,
		)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			server.Close()
			return err
		}

		return nil
	}
}
