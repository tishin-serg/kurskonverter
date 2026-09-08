# BTC Route Bot

Telegram-бот на Go 1.27.1: сравнивает расходы RUB для получения заданного BTC на внешний адрес. Считает маршруты от результата назад, проходит стакан и учитывает комиссии, ограничения и свежесть данных. Бот не выполняет сделки.

**Статус: все три маршрута реализованы и проверены на реальных API.** Настройка описана в [BYBIT_SETUP.md](BYBIT_SETUP.md) и [ROUTES_SETUP.md](ROUTES_SETUP.md). Wallet использует явно отмеченный резервный тариф вывода; BestChange исключает предложения с неподтверждёнными дополнительными комиссиями. Это не завершённый production rollout. GHCR, GitHub Actions и VPS здесь не запускались: remote и доступы не настроены.

CI/CD по примеру magnetto: push/merge в main запускает проверки, публикацию GHCR и деплой. Настройка VDS_* secrets и сервера: [GITHUB_SETUP.md](GITHUB_SETUP.md).

## Архитектура

```text
Providers → background workers → MarketSnapshot → Route engine → Telegram UI
                                      ↑                         ↓
                               atomic publication             SQLite
```

```text
cmd/bot/                  запуск, version, healthcheck, demo-quote
internal/
  domain/                 decimal-модели
  money/                  стакан, точное округление, P2P-фильтры
  route/                  обратные шаги маршрутов, ranking, partial results
  provider/
    bybit/, okx/          REST books/instruments; приватные комиссии и сети
    wallet/, bestchange/  официальные API с проверкой контрактов, без scraping
    httpclient/           общий HTTP transport, retries, validation
    demo/                 отдельно маркированные вымышленные данные
  market/                 immutable publication и независимые workers
  telegram/               команды, настройки, сохранённые подробности
  storage/migrations/     embedded SQL migrations
  app/                    wiring, /health, /ready, /metrics
  config/                 environment validation
deploy/                   compose.yaml, deploy.sh с rollback
scripts/                  Windows checks, сценарные тесты deploy
.github/workflows/        ci.yml, release.yml, deploy.yml
```

Нет float64 в денежных расчётах, Redis, ORM или тяжёлого web framework. Pure-Go SQLite позволяет CGO_ENABLED=0 production build. Race tests требуют C toolchain только при тестировании.

## Маршруты и источники

| Маршрут | Расчёт / demo | Live |
|---|---|---|
| Bybit P2P USDT → spot BTC → withdrawal | работает | официальный adapter реализован; нужны ключи и advertiser access |
| Wallet P2P GRAM → withdrawal → OKX GRAM/USDT → BTC → withdrawal | работает | P2P API и OKX live; опубликованный тариф вывода Wallet с предупреждением |
| BestChange RUB → внешний BTC | работает | официальный v2 API; безналичный RUB → BTC, проверяемые комиссии и лимиты |

Публичные Bybit/OKX клиенты реализуют REST books и instrument precision. Unit tests используют httptest, а не реальные биржи. WebSocket заменён разрешённым REST polling каждые 5s. Наличие торгового инструмента определяется ответом API; GRAM никогда автоматически не заменяется другой монетой. `BYBIT_P2P_SOURCE=official|web` выбирает отдельные adapters: официальный adapter реализован; **web возвращает ErrNotConfigured**. Undocumented endpoints не вызываются.

Контракты, ограничения и первичные источники описаны в [ROUTES_SETUP.md](ROUTES_SETUP.md).

## Расчёт и точность

Withdrawal: требуемый выход округляется вверх до withdrawal step, затем прибавляется фиксированная комиссия. Для покупки BTC комиссия удерживается в получаемом BTC: `grossBTC = ceilStep(requiredBTC / (1 - takerFee))`. Стоимость покупки — проход asks; продажа GRAM для получения USDT инвертирует bids и учитывает комиссию в USDT. Это модель исполнения по доступному стакану; будущие движения цены и задержки переводов она не гарантирует.

`QuoRem` используется для точного округления рациональной величины вверх без глобальной precision `decimal.Div`. До последнего шага нет раннего округления рублей. RUB платёж округляется вверх до копейки. Tick size проверяется, base step и минимумы сделки учитываются. Излишки промежуточных активов могут остаться на бирже; на адрес поступит не меньше целевого BTC, с явным предупреждением при округлении withdrawal.

P2P выбирает один подходящий offer по RUB/asset, payment method, min/max, reserve, completion rate и order count. Несколько объявлений не агрегируются. BestChange выбирает самый дешёвый полностью покрывающий запрос вариант. Ошибка одного маршрута не ломает остальные. Ranking содержит абсолютную и процентную разницу с лучшим.

Скрытых констант нет. В локальной конфигурации включён явный BYBIT_P2P_TAKER_FEE_FALLBACK=0: Bybit возвращает пустые поля комиссии RUB/USDT, поэтому эта котировка всегда маркируется IsDegraded. Торговая комиссия и комиссия вывода поступают из API. Поле `Fee.Fallback` поддерживается ядром: котировка получает `IsDegraded` и предупреждение. Только demo содержит явно вымышленные фиксированные значения.

## Локальный запуск

Создайте бота через Telegram @BotFather (`/newbot`), сохраните token вне git. Команды приложения: `/start`, `/help`, `/settings`, `/settings <payment ID>`, `/settings any`. Сообщение: `0.01` или `0.01 BTC`; запятая тоже допустима, максимум 8 десятичных знаков. Только private chats. Кнопки раскрывают сохранённый расчёт и создают свежий расчёт из текущего snapshot.

Приложение читает environment, **не загружает .env автоматически**. `.env.example` — образец для Compose или настройки переменных процессу. Не коммитьте token/API keys. `BYBIT_API_KEY` и `BYBIT_API_SECRET` читаются Bybit adapter. `OKX_API_*`, `WALLET_API_KEY`, `BESTCHANGE_API_KEY` используются соответствующими адаптерами. PowerShell scripts/run.ps1 безопасно загружает локальный .env.

```bash
go mod download
make test
make test-race
make lint
make vuln
make build
DATA_MODE=demo ./bin/bot
# другой терминал
curl -f http://127.0.0.1:8080/health
curl -f http://127.0.0.1:8080/ready
./bin/bot demo-quote
```

Для demo с Telegram задайте `TELEGRAM_BOT_TOKEN`. Без token demo запускает workers и HTTP для локального smoke test. Live требует token и настроенный Bybit API с доступом P2P; без них соответствующий маршрут недоступен. Demo никогда не включается автоматически при ошибке live API.

Windows PowerShell:

```powershell
./scripts/check.ps1 test
./scripts/check.ps1 lint
./scripts/check.ps1 coverage
./scripts/check.ps1 vuln
./scripts/check.ps1 build
$env:DATA_MODE='demo'
./bin/bot.exe
```

Скрипт держит Go/staticcheck caches в `.cache/`. Для `./scripts/check.ps1 race`: установите `CGO_ENABLED=1`, `CC` на совместимый MinGW clang/gcc и добавьте его bin в PATH. При разработке проверено с переносимым LLVM-MinGW 20260826 UCRT (SHA256 проверен); compiler не входит в репозиторий.

## Snapshot и отказоустойчивость

Workers стартуют независимо через errgroup. HTTP выполняется вне mutex. Публикация через `atomic.Pointer`, writer и reader получают глубокие копии коллекций. Ошибка refresh сохраняет последний успешный источник с прежним timestamp; возраст никогда не обнуляется при ошибке.

Стаканы старше 15s, P2P/BestChange старше 60s, fees/instruments старше 15min исключаются. Источники с датой в будущем также исключаются. Poll defaults: Bybit P2P 20s, Wallet 30s, BestChange 20s, fees/instruments 5min, books 5s. Environment позволяет 1s..1h; слишком большой interval приводит к исключению stale данных, а не маскирует их.

HTTP client переиспользует соединения, ограничивает ответ 2 MiB, проверяет status и JSON, уважает context. GET retry: до 3 попыток на 429/5xx, exponential backoff, ограниченный Retry-After; остальные ошибки возвращаются. Credentials и response bodies не логируются. Telegram errors также редактируются, чтобы token из URL не попал в лог.

`/health`: жив процесс и build info. `/ready`: запущен worker supervisor, Telegram инициализирован (либо explicit demo без Telegram), SQLite отвечает и хотя бы один маршрут может рассчитать контрольные 0.01 BTC. Один optional источник не влияет на готовность при наличии другого маршрута. Это консервативная readiness probe: отсутствие offer для 0.01 может дать 503, даже если другая сумма доступна. Без настроенного рабочего источника ожидается 503; Bybit может обеспечить readiness независимо от остальных источников.

`/metrics`: provider_errors_total и provider_last_success_timestamp в Prometheus text format. Расширенные duration/quote metrics пока не реализованы. JSON slog logs. Healthcheck CLI использует локальный порт из HTTP_ADDR (default 8080), закреплённый Compose. HTTP endpoints не публикуются наружу VPS.

## SQLite и migrations

`users`, `user_settings`, `quote_history`; decimals хранятся строками/JSON. История ограничена последними 100 расчётами на пользователя, запрос подробностей проверяет владельца. WAL, busy_timeout, foreign_keys, один DB connection. Embedded migrations применяются в транзакции при startup; версия записывается один раз. Ошибка migration останавливает startup. Начальная миграция additive/idempotent; последующие должны сохранять совместимость предыдущего binary для rollback. Приложение не исполняет destructive migrations.

Перед изменениями схемы делайте SQLite-consistent backup (`sqlite3 .backup` или остановка контейнера и копирование всего volume), не копируйте только live db без WAL. Retention по возрасту пользователей и автоматический backup требуют операционной настройки.

## Docker

```bash
make docker-build
docker run --rm -p 127.0.0.1:8080:8080 -e DATA_MODE=demo \
  -v btc-route-data:/data btc-route-bot:local
```

Multi-stage: Go builder → Alpine runtime с CA certificates, UID/GID 10001. Volume `/data`, read-only rootfs в Compose, dropped capabilities, 512 MiB / 1 CPU, GOMEMLIMIT=384MiB. Это заданные лимиты, **не результат нагрузочного теста**. Docker Engine отсутствовал в среде разработки, поэтому локальный image build не подтверждён; CI проверяет его на каждом PR.

## CI, release, GHCR

`ci.yml`: PR и push main, без secrets. gofmt, vet, staticcheck, обычные/race tests, coverage artifact, govulncheck, static binary, deploy scenario tests, Docker build. Те же команды в Makefile. Coverage ядра лучше инфраструктуры; провайдеры покрыты HTTP fixtures. Dependabot еженедельно для gomod, actions, Docker, minor/patch Go updates сгруппированы.

`release.yml`: тег `vX.Y.Z` → reusable CI → binaries linux/amd64 и linux/arm64 + checksums → multiarch image GHCR → GitHub Release с `image.txt` → reusable deploy. Actions используют stable major versions, минимальные permissions и GITHUB_TOKEN. Нет `pull_request_target`, нет публикации PR image. Семантические теги без prerelease; публикация main image намеренно не включена.

```bash
git tag v0.1.0
git push origin v0.1.0
```

Image: `ghcr.io/<owner>/<repository>` (lowercase). Tags: `v1.2.3`, `1.2.3`, `1.2`, `1`, `latest`, `sha-<full SHA>`. Deploy использует **digest `@sha256:...`**, а не mutable tags. Binary `bot version` отдаёт version/commit/date из ldflags. Release artifact `image.txt` содержит exact digest для manual rollback/deploy.

Перед первым реальным деплоем заполните VDS_* secrets и подготовьте сервер по [GITHUB_SETUP.md](GITHUB_SETUP.md). Создание workflows само по себе не означает зелёный GitHub run или опубликованный image.

## Подготовка VPS и GitHub

Linux VPS: Docker Engine, Compose v2, Bash, flock, SSH. Пользователь `kursdeploy` получает только forced command для деплоя; настройка — в GITHUB_SETUP.md. Docker group и общий sudo запрещены. Подготовьте `/opt/kurskonverter`, скопируйте **только** `deploy/compose.yaml` как `compose.yaml` и создайте `.env` на VPS. Repository и Go compiler серверу не нужны. Restrict .env mode 600; ограничьте SSH и не открывайте 8080 во внешний интернет.

Создайте GitHub Environment `production`, ограничьте разрешённые deployment branches/tags, по желанию включите reviewers. Secrets:

| Secret | Значение |
|---|---|
| VDS_HOST | DNS/IP VPS |
| VDS_USER | Только `kursdeploy` |
| VDS_SSH_PRIVATE_KEY | выделенный private SSH key |
| VDS_PORT | SSH port, default 22 |
| VDS_KNOWN_HOSTS | проверенный host key в known_hosts формате, включая `[host]:port` при нестандартном порте |

Проверяйте fingerprint по доверенному каналу. Workflow не использует `StrictHostKeyChecking=no` или автоматическое доверие ssh-keyscan.

Сервер скачивает публичный GHCR package без токена. Для private package администратор отдельно настраивает read-only registry credential. App credentials находятся только в VPS .env, не передаются в Docker build.

## Deploy и rollback

`deploy.yml` вызывается после успешной публикации release; есть workflow_dispatch с exact digest и версией. `environment: production`, `concurrency: production`, cancel-in-progress=false. На VPS дополнительный flock защищает от одновременного ручного deploy.

Процесс: проверить digest текущего repository → ограниченная SSH-команда → pull → сохранить `previous-image` → Compose up → дождаться Docker health и `bot healthcheck ready` → атомарно записать `current-image`. Проверки ограничены 24 попытками по 5 секунд. Job summary содержит image version, image commit label, digest и результат. Сервер не делает git pull и не компилирует код.

Если pull падает, текущий контейнер не меняется. Если новая версия не запускается/не готова, script восстанавливает предыдущий exact image, снова проверяет здоровье и завершает workflow ошибкой. Если предыдущего image нет, останавливает неудачный первый deploy. Провал rollback выводит требование вмешательства. Миграции должны быть backward compatible; автоматического отката данных нет.

Ручной rollback: предпочтительно workflow_dispatch с digest из предыдущего Release `image.txt`. Либо с локальной машины, где есть скрипт:

```bash
ssh deploy@vps 'cat /opt/kurskonverter/previous-image'
# Подставьте проверенный digest из вывода; при private GHCR сначала выполните login.
ssh deploy@vps "bash -s -- 'ghcr.io/owner/repo@sha256:...'" < deploy/deploy.sh
```

Для обычных ручных compose-команд загрузите текущий image: `export BOT_IMAGE=$(cat current-image)` из `/opt/kurskonverter`. `current-image`/`previous-image` — служебные файлы deploy, не secrets. Не запускайте две копии Telegram polling с одним token.

## Добавление provider / route

1. Реализуйте небольшой interface в `internal/provider` и отдельный client через общий HTTP transport.
2. Нормализуйте asset/network, fee side, status, limits и timestamp. Не превращайте missing fields в нулевые комиссии. Добавьте httptest fixtures по подтверждённой спецификации.
3. Зарегистрируйте независимый worker в app; обновляйте snapshot только после успешной валидации ответа.
4. Новый линейный маршрут можно описать `route.Path` с обратимыми `Step`, либо реализовать `Calculator` для нестандартного пути. Existing routes не меняются.
5. Добавьте финансовые fixtures, stale/error/rounding tests и проверку реального аккаунта перед включением live.

## Branch protection

Для main: запрет direct push, только PR, required CI `checks`, branch up to date, минимум один review при нескольких участниках. Ограничьте создание release tags доверенным maintainers. Repository settings автоматически не изменяются.

## Пример 0.01 BTC

Вымышленные demo-данные, не рыночная рекомендация:

```text
🧪 ДЕМО — вымышленные данные
💰 Получить 0.01000000 BTC

🥇 BestChange
88000.00 ₽

🥈 Wallet → GRAM → OKX
89663.05 ₽
+1663.05 ₽ / +1.89%

🥉 Bybit P2P → USDT → BTC
90554.41 ₽
+2554.41 ₽ / +2.90%
```

## Ограничения первой реализации

Live adapters перечислены выше; контрактов и ключей недостаточно для production acceptance. Нет websocket, order aggregation, execution/reservation, адресов, автоматических сделок и guarantees будущей цены. Не реализованы процентные withdrawal и нестандартные fee assets/tiers; adapter обязан отклонить неподдерживаемую модель. User-level rate limiting и telemetry Telegram polling health требуют усиления перед публичным запуском. Проверки на реальном Telegram, Docker/GHCR/VPS и нагрузочные тесты остаются обязательным этапом при доступной инфраструктуре.





