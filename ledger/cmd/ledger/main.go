package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"example.com/finance-services/ledger"
)

func run() error {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
	)
	defer stop()

	_, closeFn, err := ledger.New(ctx)
	if err != nil {
		return err
	}
	defer closeFn()

	log.Print(
		"Ledger готов. Для HTTP запустите Gateway. Ctrl+C — выход.",
	)

	<-ctx.Done()
	return nil
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
