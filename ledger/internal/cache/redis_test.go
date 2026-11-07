package cache

import (
	"context"
	"testing"
	"time"

	"example.com/finance-services/ledger/internal/domain"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestReportsCache(t *testing.T) {
	server := miniredis.RunT(t)

	client := redis.NewClient(&redis.Options{
		Addr:       server.Addr(),
		MaxRetries: -1,
	})
	defer client.Close()

	c := New(client)
	ctx := context.Background()

	report := domain.ReportSummary{
		From:  "2026-10-01",
		To:    "2026-10-31",
		Total: 450,
		Items: []domain.ReportItem{
			{Category: "еда", Amount: 450},
		},
	}

	if _, hit, err := c.Get(ctx, report.From, report.To); err != nil || hit {
		t.Fatal(hit, err)
	}

	if err := c.Set(ctx, report, 30*time.Second); err != nil {
		t.Fatal(err)
	}

	got, hit, err := c.Get(ctx, report.From, report.To)
	if err != nil || !hit || got.Total != 450 || len(got.Items) != 1 {
		t.Fatal(got, hit, err)
	}

	if ttl := server.TTL(key(report.From, report.To)); ttl != 30*time.Second {
		t.Fatal(ttl)
	}

	server.FastForward(31 * time.Second)

	if _, hit, err := c.Get(ctx, report.From, report.To); err != nil || hit {
		t.Fatal(hit, err)
	}

	server.Set(key(report.From, report.To), "broken")

	if _, hit, err := c.Get(ctx, report.From, report.To); err == nil || hit {
		t.Fatal("corrupt cache accepted")
	}

	server.Close()

	if _, _, err := c.Get(ctx, report.From, report.To); err == nil {
		t.Fatal("connection error ignored")
	}
}
