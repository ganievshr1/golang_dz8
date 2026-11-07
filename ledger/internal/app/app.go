package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"sync"
	"time"

	reportcache "example.com/finance-services/ledger/internal/cache"
	"example.com/finance-services/ledger/internal/repository/pg"
	"example.com/finance-services/ledger/internal/service"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/redis/go-redis/v9"
)

func envOr(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func databaseDSN() string {
	if value := os.Getenv("DATABASE_URL"); value != "" {
		return value
	}

	u := url.URL{
		Scheme: "postgres",
		User: url.UserPassword(
			envOr("DB_USER", "postgres"),
			envOr("DB_PASS", "postgres"),
		),
		Host: net.JoinHostPort(
			envOr("DB_HOST", "localhost"),
			envOr("DB_PORT", "5432"),
		),
		Path: "/" + envOr("DB_NAME", "cashapp"),
	}

	q := u.Query()
	q.Set("sslmode", "disable")
	u.RawQuery = q.Encode()

	return u.String()
}

func New(ctx context.Context) (service.Service, func() error, error) {
	redisDB, err := strconv.Atoi(envOr("REDIS_DB", "0"))
	if err != nil || redisDB < 0 {
		return nil, nil, errors.New(
			"REDIS_DB должен быть целым неотрицательным числом",
		)
	}

	db, err := sql.Open("pgx", databaseDSN())
	if err != nil {
		return nil, nil, fmt.Errorf(
			"PostgreSQL: открыть подключение: %w",
			err,
		)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(30 * time.Minute)

	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	if err := db.PingContext(startup); err != nil {
		db.Close()

		return nil, nil, fmt.Errorf(
			"PostgreSQL недоступен; проверьте DATABASE_URL: %w",
			err,
		)
	}

	client := redis.NewClient(&redis.Options{
		Addr:                  envOr("REDIS_ADDR", "localhost:6379"),
		DB:                    redisDB,
		Password:              os.Getenv("REDIS_PASSWORD"),
		DialTimeout:           2 * time.Second,
		ReadTimeout:           time.Second,
		WriteTimeout:          time.Second,
		MaxRetries:            -1,
		ContextTimeoutEnabled: true,
	})

	if err := client.Ping(startup).Err(); err != nil {
		client.Close()
		db.Close()

		return nil, nil, fmt.Errorf(
			"Redis недоступен; проверьте REDIS_*: %w",
			err,
		)
	}

	budgets, expenses, work := pg.New(db)

	svc := service.New(
		budgets,
		expenses,
		work,
		reportcache.New(client),
	)

	var once sync.Once
	var closeErr error

	closeFn := func() error {
		once.Do(func() {
			closeErr = errors.Join(client.Close(), db.Close())
		})

		return closeErr
	}

	log.Print("Ledger: подключение к PostgreSQL установлено")
	log.Print("Ledger: подключение к Redis установлено")

	return svc, closeFn, nil
}
