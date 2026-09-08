# Проверки 2026-09-07

## Дополнительная live-проверка Bybit и Telegram

После настройки прав API-ключа успешно получены торговая комиссия аккаунта, условия вывода BTC и P2P объявления. Реальный P2P API возвращает пустые buyFeeRate/sellFeeRate; включена явная настройка BYBIT_P2P_TAKER_FEE_FALLBACK=0 согласно опубликованному тарифу RUB. Котировка помечена IsDegraded и содержит предупреждение. Ненулевые/некорректные явно возвращённые ставки fallback не заменяет.

Контрольный расчёт 0.01000000 BTC: 68 559.78 RUB, 826 валидных объявлений на момент проверки (не текущая фиксированная цена). Telegram getMe успешен для @kurskonverter_bot. Бот запущен локально в live, /health и /ready возвращают 200 на 127.0.0.1:18089. Инициализация Telegram получила таймаут 15s вместо стандартного короткого окна. Автозапуска после перезагрузки нет; VPS/GHCR ещё не развёрнуты.

Тесты fallback, пагинации, расчётов и race проходят. Значения credentials не записывались в отчёт/логи; .env исключён git.

## Исходные проверки и ограничения инфраструктуры

Среда: Windows amd64, Go 1.27.1 (автоматически загруженный toolchain), pure-Go SQLite. Для race использован переносимый LLVM-MinGW 20260826 UCRT из официального release, SHA256 проверен. Системная установка компилятора не выполнялась.

| Проверка | Результат |
|---|---|
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `gofmt`, `go vet ./...` | PASS |
| staticcheck v0.8.1 | PASS |
| govulncheck v1.7.0 | No vulnerabilities found |
| Windows binary, CGO_ENABLED=0 | PASS |
| Linux amd64 / arm64 binary, CGO_ENABLED=0 | PASS |
| `bot version`, `bot demo-quote` | PASS |
| `/health`, `/ready`, `/metrics` demo smoke | PASS на отдельном локальном порту |
| CLI healthcheck / readiness | PASS |
| actionlint v1.7.12 | PASS; внешние shellcheck/pyflakes отключены |
| Bash syntax deploy.sh | PASS |
| Deploy success, failed readiness → rollback, failed first deployment, failed pull, failed rollback → operator alert | PASS, mock Docker |

Покрытие исходной версии до добавления Bybit live adapters: money 93.8%, route 83.7%, market 96.7%, storage 75.3%, telegram 62.4%, config 100%, всего 57.0%. Неподключённые adapters явно уменьшают общий процент. Coverage artifact создаётся CI; локальный coverage.out игнорируется git.

Первый security scan на предустановленном Go 1.26.4 выявил уязвимости стандартной библиотеки. После перехода на актуальный стабильный Go 1.27.1 повторный scan чистый. Первоначальная невозможность race из-за отсутствия CGO устранена локальным portable toolchain.

## Что не подтверждено

- Docker build и runtime контейнера: Docker Engine недоступен в этой среде. В CI есть обязательный `make docker-build`.
- Реальный GitHub CI run, GHCR push, GitHub Release и VPS deployment: git remote и инфраструктурные доступы не настроены. Workflows проверены статически; shell rollback проверен с mock Docker, что не заменяет VPS acceptance.
- Реальный Telegram: token не предоставлен; UI проверен через httptest, без сообщений реальным пользователям.
- Bybit official P2P и fees реализованы и проверены через httptest полного маршрута; приватный live-запуск ожидает ключи и advertiser access. Wallet, OKX fees и BestChange всё ещё возвращают ErrNotConfigured.
- Нагрузка 1 vCPU / 512 MiB и восстановление SQLite из backup не тестировались.

Таким образом, локальная реализация и инфраструктурные конфигурации проверены в доступных пределах. Полный production Definition of Done ещё не достигнут. Дальнейшие шаги и все TODO приведены в README.md.


## Остальные маршруты — 2026-09-08

- Wallet integration API: live SELL RUB/TON, нормализуется в GRAM; OKX GRAM-USDT listing и нативная сеть депозита подтверждены API.
- OKX: HMAC-подпись, персональные taker fees, режим валюты комиссии, лимиты spot, доступность/минимум депозита GRAM и комиссия/минимум/точность вывода BTC.
- BestChange v2: live Т-Банк RUB → BTC. Предложения с неописанными дополнительными комиссиями исключаются; названия обменников необязательны, есть ID и ссылка.
- `check-routes any`: 2026-09-07 22:02:56 UTC получены все три реальные котировки. Фильтр `.env` Tbank не изменялся. С фильтром Tbank рассчитались Wallet и BestChange; Bybit не объявляет отдельный способ оплаты Т-Банк.
- Обычные тесты, `go test -race ./...`, `go vet ./...`, staticcheck v0.8.1 прошли. Новые зависимости не добавлялись.
- Остаточные ограничения: Wallet withdrawal fee = явный degraded fallback 0.05 GRAM; OKX feeType=1 отклоняется; у BestChange используется проверяемое подмножество предложений. GHCR/VPS rollout по-прежнему не выполнялся.

## Настройки P2P — 2026-09-08

Добавлены `/banks`, отдельные настройки Bybit/Wallet и миграция 002. Список способов динамический, выбор по устойчивому идентификатору; устаревшие callback не меняют настройку. Исправлен сброс `/settings any` при серверном PAYMENT_METHOD. Unit tests, vet и race tests telegram/storage/route прошли. Направление BestChange меню P2P не меняет.
