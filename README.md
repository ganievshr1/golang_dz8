# Finance Services

Учебный проект на Go 1.23+, PostgreSQL 16 и Redis 7.

HTTP API работает на порту 8080. Все суммы в Go и JSON указаны в целых
копейках. PostgreSQL хранит суммы в рублях в NUMERIC(14,2).
Например, amount=450 означает 4 рубля 50 копеек.
Требуется 64-битный Go.

## Структура и зависимости

| Путь | Назначение |
| --- | --- |
| ledger/internal/domain | Сущности, валидация, ошибки и интерфейсы |
| ledger/internal/repository/pg | SQL, репозитории PostgreSQL и транзакции |
| ledger/internal/service | Бизнес-логика, правило бюджета и отчёты |
| ledger/internal/cache | Кэш отчётов Redis |
| ledger/internal/app | Конфигурация, подключения и сборка сервиса |
| ledger/ledger.go | Публичные типы, интерфейс Service и фабрика New |
| ledger/cmd/ledger | Проверка запуска Ledger |
| gateway/internal/api | DTO, HTTP-обработчики и middleware |

Домен не зависит от SQL и HTTP.
Сервис зависит от доменных интерфейсов, а не от PostgreSQL или Redis.
Репозитории реализуют доменные интерфейсы.
Пакет app создаёт подключения и собирает сервис.

Gateway получает сервис через ledger.New(ctx).
Фабрика возвращает Service, closeFn и error.
После успешной инициализации вызывается defer closeFn().
Сервис передаётся в api.NewHandler(svc, logger).

Обработчики вызывают методы интерфейса и передают r.Context().
Валидация сущностей и правило бюджета выполняются в сервисе.
Глобальных подключений нет. Миграции фабрика не запускает.

Проверка бюджета и вставка выполняются в одной SQL-транзакции.
Блокировка категории защищает лимит при параллельных запросах.
Если бюджета категории нет, транзакции разрешены.
LoadBudgets сохраняет массив бюджетов атомарно.

## PostgreSQL и Redis

Команды PowerShell выполняются из корня finance-services.

```powershell
docker compose up -d --wait

$env:DATABASE_URL = 'postgres://postgres:postgres@127.0.0.1:15432/cashapp?sslmode=disable'
$env:REDIS_ADDR = '127.0.0.1:6379'
$env:REDIS_DB = '0'
$env:REDIS_PASSWORD = ''
```

PostgreSQL доступен с компьютера на порту 15432,
внутри контейнера — на 5432.

DATABASE_URL имеет приоритет. Без неё используются:

- DB_HOST=localhost;
- DB_PORT=5432;
- DB_USER=postgres;
- DB_PASS=postgres;
- DB_NAME=cashapp.

При использовании Compose без DATABASE_URL задайте DB_PORT=15432.

Настройки Redis:

- REDIS_ADDR=localhost:6379;
- REDIS_DB=0;
- REDIS_PASSWORD необязателен.

Переменные окружения нужно задавать в каждом новом окне PowerShell.

Пул PostgreSQL: MaxOpenConns=10, MaxIdleConns=5,
ConnMaxLifetime=30 минут.

## Зависимости и миграции

```powershell
Push-Location ledger
go mod tidy
Pop-Location

Push-Location gateway
go mod tidy
Pop-Location

go install github.com/pressly/goose/v3/cmd/goose@v3.24.1

$goose = Join-Path (go env GOPATH) 'bin\goose.exe'

& $goose -dir ./ledger/migrations postgres $env:DATABASE_URL up
& $goose -dir ./ledger/migrations postgres $env:DATABASE_URL status
```

Если команда завершилась ошибкой, исправьте её до следующего шага.

Откат последней миграции:

```powershell
& $goose -dir ./ledger/migrations postgres $env:DATABASE_URL down
```

Откат удаляет соответствующие таблицы и их данные.
Для обычного перезапуска откатывать миграции не нужно.

## Запуск Gateway

После успешных миграций, в окне с заданными env:

```powershell
Push-Location gateway
go run .
Pop-Location
```

Gateway создаёт экземпляр Ledger в своём процессе.
Отдельный процесс Ledger для работы HTTP API не требуется.

Проверка:

```powershell
curl.exe -i http://localhost:8080/ping
```

Ожидается 200 и {"message":"pong"}.

Остановка — Ctrl+C.

Отдельная проверка запуска Ledger:

```powershell
Push-Location ledger
go run ./cmd/ledger
Pop-Location
```

Эта команда проверяет подключения и ожидает Ctrl+C.
HTTP обслуживает Gateway.

## API

| Метод и путь | Результат |
| --- | --- |
| GET /ping | 200, проверка доступности |
| POST /api/budgets | 201, создание или обновление бюджета |
| GET /api/budgets | 200, массив бюджетов |
| POST /api/transactions | 201, транзакция с ID |
| GET /api/transactions | 200, массив транзакций |
| GET /api/reports/summary?from=YYYY-MM-DD&to=YYYY-MM-DD | 200, отчёт |

Пустые списки возвращаются как [].

Все ответы имеют Content-Type: application/json; charset=utf-8.

Ошибки имеют формат {"error":"..."}:

- 400 — некорректный JSON, валидация или период отчёта;
- 409 — {"error":"budget exceeded"};
- 500 — {"error":"internal server error"};
- 404 — неизвестный путь;
- 405 — неподдерживаемый метод.

## Проверка cURL в PowerShell

Команды выполняются во втором окне при работающем Gateway.
Каждый запуск создаёт отдельную тестовую категорию.

```powershell
$OutputEncoding = [System.Text.UTF8Encoding]::new($false)
$category = 'check_' + [guid]::NewGuid().ToString('N')

@{category=$category;limit=1000} |
    ConvertTo-Json -Compress |
    curl.exe -i -X POST http://localhost:8080/api/budgets -H 'Content-Type: application/json' --data-binary '@-'

@{amount=450;category=$category;description='lunch';date='2026-11-07'} |
    ConvertTo-Json -Compress |
    curl.exe -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' --data-binary '@-'

@{amount=600;category=$category;description='over limit';date='2026-11-07'} |
    ConvertTo-Json -Compress |
    curl.exe -i -X POST http://localhost:8080/api/transactions -H 'Content-Type: application/json' --data-binary '@-'

curl.exe -i http://localhost:8080/api/budgets
curl.exe -i http://localhost:8080/api/transactions

curl.exe -i 'http://localhost:8080/api/reports/summary?from=2026-11-01&to=2026-11-30'
curl.exe -i 'http://localhost:8080/api/reports/summary?from=2026-11-01&to=2026-11-30'
```

Ожидаемые статусы: 201, 201, 409, затем 200.

Для новой категории сохраняется одна транзакция на 450 копеек.
Транзакция на 600 копеек не сохраняется.

Отчёты кэшируются на 30 секунд. В логах Gateway видны cache miss и cache hit.
После записи отчёт может оставаться прежним до истечения TTL.
При недоступности Redis после запуска отчёт считается через PostgreSQL.
Список бюджетов не кэшируется.

Для проверки сохранности данных остановите Gateway через Ctrl+C,
запустите его снова и повторите GET-запросы.
Данные и ID должны сохраниться.

## Автоматические тесты

Без внешних сервисов:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/check.ps1
```

С работающими PostgreSQL и Redis:

```powershell
$env:TEST_DATABASE_URL = $env:DATABASE_URL
$env:TEST_REDIS_DB = '15'

powershell -NoProfile -ExecutionPolicy Bypass -File ./scripts/check.ps1 -Integration
```

Интеграционный тест создаёт отдельную схему PostgreSQL и удаляет её после проверки.
Используйте отдельную тестовую БД Redis 15.

Проверяются:

- валидация без обращения к хранилищу;
- UPSERT бюджета;
- точный лимит и отказ при превышении;
- отсутствие записи при отказе;
- порядок списков;
- преобразование копеек и рублей;
- Redis cache hit/miss и TTL;
- сохранность после повторного создания сервиса;
- конкурентные записи;
- HTTP-контракты и формат ошибок.

Успешный итог:

```text
[OK] All selected checks passed.
```