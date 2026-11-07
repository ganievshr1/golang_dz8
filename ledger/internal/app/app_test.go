package app

import (
	"context"
	"errors"
	"net/url"
	"testing"
)

func TestConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://override")

	if databaseDSN() != "postgres://override" {
		t.Fatal("DATABASE_URL must win")
	}

	t.Setenv("DATABASE_URL", "")
	t.Setenv("DB_HOST", "127.0.0.1")
	t.Setenv("DB_PORT", "15432")
	t.Setenv("DB_USER", "user@name")
	t.Setenv("DB_PASS", "p@ss:/?")
	t.Setenv("DB_NAME", "cashapp")

	u, err := url.Parse(databaseDSN())
	if err != nil {
		t.Fatal(err)
	}

	pass, _ := u.User.Password()

	if u.Host != "127.0.0.1:15432" ||
		u.User.Username() != "user@name" ||
		pass != "p@ss:/?" {
		t.Fatal("invalid DSN escaping")
	}

	t.Setenv("REDIS_DB", "-1")

	if s, closeFn, err := New(context.Background()); err == nil ||
		s != nil ||
		closeFn != nil {
		t.Fatal("invalid config accepted")
	}
}

func TestFactoryHonorsCanceledContext(t *testing.T) {
	t.Setenv(
		"DATABASE_URL",
		"postgres://postgres:postgres@127.0.0.1:1/cashapp?sslmode=disable",
	)
	t.Setenv("REDIS_DB", "0")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s, closeFn, err := New(ctx)

	if !errors.Is(err, context.Canceled) || s != nil || closeFn != nil {
		t.Fatalf(
			"service=%v closeNil=%v err=%v",
			s,
			closeFn == nil,
			err,
		)
	}
}
