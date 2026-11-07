package cache

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"example.com/finance-services/ledger/internal/domain"
	"github.com/redis/go-redis/v9"
)

type Reports struct {
	client *redis.Client
}

func New(client *redis.Client) *Reports {
	return &Reports{client}
}

var _ domain.ReportCache = (*Reports)(nil)

func key(from, to string) string {
	return "report:summary:" + from + ":" + to
}

func (c *Reports) Get(
	ctx context.Context,
	from, to string,
) (domain.ReportSummary, bool, error) {
	data, err := c.client.Get(ctx, key(from, to)).Bytes()

	if errors.Is(err, redis.Nil) {
		return domain.ReportSummary{}, false, nil
	}

	if err != nil {
		return domain.ReportSummary{}, false, err
	}

	var result domain.ReportSummary

	if err := json.Unmarshal(data, &result); err != nil {
		return result, false, err
	}

	if result.From != from || result.To != to || result.Items == nil {
		return domain.ReportSummary{}, false, errors.New(
			"некорректное содержимое кэша",
		)
	}

	return result, true, nil
}

func (c *Reports) Set(
	ctx context.Context,
	result domain.ReportSummary,
	ttl time.Duration,
) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}

	return c.client.Set(
		ctx,
		key(result.From, result.To),
		data,
		ttl,
	).Err()
}
