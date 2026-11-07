# Ledger

Ledger хранит бюджеты и транзакции в PostgreSQL.
Отчёты кэшируются в Redis на 30 секунд.
Суммы в интерфейсе сервиса указаны в целых копейках.

## Слои

- internal/domain — сущности, Validate, ошибки и контракты;
- internal/repository/pg — SQL и реализация репозиториев;
- internal/service — бизнес-правила;
- internal/cache — реализация кэша Redis;
- internal/app — конфигурация и сборка зависимостей;
- ledger.go — публичный интерфейс и фабрика.

ledger.New(ctx) возвращает Service, closeFn и error.
После успешного вызова используйте defer closeFn().
Все методы сервиса принимают context.Context.

Глобальных подключений нет.
Фабрика не запускает миграции.

## Подготовка и запуск

Команды PostgreSQL, Redis, Goose и cURL:
[корневой README](../README.md).

После запуска контейнеров и применения миграций,
из корня проекта:

```powershell
$env:DATABASE_URL = 'postgres://postgres:postgres@127.0.0.1:15432/cashapp?sslmode=disable'
$env:REDIS_ADDR = '127.0.0.1:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''

Push-Location ledger
go test -count=1 -cover ./...
go vet ./...
go run ./cmd/ledger
Pop-Location
```

cmd/ledger проверяет подключение и ожидает Ctrl+C.
HTTP обслуживает Gateway, создающий собственный экземпляр сервиса.

## Интеграционные тесты

Из корня проекта при работающих PostgreSQL и Redis:

```powershell
$env:TEST_DATABASE_URL = $env:DATABASE_URL
$env:TEST_REDIS_DB = '15'

powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/check.ps1 -Integration
```

Тест создаёт и удаляет отдельную схему PostgreSQL.
Рабочие таблицы приложения не очищаются.
Redis DB 15 должна использоваться для тестов.