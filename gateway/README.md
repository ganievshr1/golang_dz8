# Gateway

HTTP API на порту 8080.

Gateway импортирует публичный пакет ledger и получает готовый сервис:

```go
svc, closeFn, err := ledger.New(ctx)
if err != nil {
    return err
}
defer closeFn()

handler := api.NewHandler(svc, logger)
```

Обработчики используют интерфейс ledger.Service.
Прямых импортов database/sql, pgx и repository нет.

DTO, маршруты, коды ответов и JSON-контракты сохранены.
Валидация сущностей и бюджетное правило выполняются в Ledger.

Gateway преобразует ошибки:

- ValidationError — 400;
- ErrInvalidPeriod — 400;
- ErrBudgetExceeded — 409;
- внутренняя ошибка — 500.

Все ответы имеют Content-Type: application/json; charset=utf-8.
Ошибки имеют формат {"error":"..."}.

## Эндпоинты

- GET /ping;
- GET/POST /api/budgets;
- GET/POST /api/transactions;
- GET /api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD.

## Запуск и тестирование

Подготовка PostgreSQL/Redis, миграции, env и примеры cURL:
[корневой README](../README.md).

После настройки env из корня проекта:

```powershell
Push-Location gateway
go test -count=1 -cover ./...
go vet ./...
go run .
Pop-Location
```

Отдельный процесс Ledger для HTTP не требуется:
Gateway создаёт сервис в своём процессе.

После Ctrl+C и повторного запуска записи и ID должны сохраниться.
Отчёты кэшируются на 30 секунд.
Логи cache miss/hit появляются в процессе Gateway.