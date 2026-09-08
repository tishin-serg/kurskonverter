# Запуск маршрута Bybit

Официальный маршрут реализован: RUB → P2P USDT → spot BTCUSDT → внешний BTC через сеть BTC. Для проверки на вашем аккаунте ещё нужны credentials и доступ P2P API.

## Подготовка

1. Проверьте статус аккаунта: официальный P2P API доступен General Advertiser и выше. [Условие Bybit](https://bybit-exchange.github.io/docs/p2p/guide).
2. Создайте system-generated HMAC API key для чтения нужных данных. Бот использует только чтение объявлений, account fee-rate и coin query-info. Торговлю и вывод средств бот не вызывает; разрешение на вывод ему не нужно. RSA keys в этой реализации не поддерживаются.
3. Укажите значения в локальном `.env`, который исключён из git:

```dotenv
DATA_MODE=live
BYBIT_P2P_SOURCE=official
BYBIT_API_KEY=ваш_ключ
BYBIT_API_SECRET=ваш_секрет
TELEGRAM_BOT_TOKEN=токен_от_BotFather
```

Не отправляйте ключи в чат. Для диагностического запуска Telegram token не нужен. Системные часы должны быть синхронизированы; подпись использует окно 5000 ms.

## Проверка в PowerShell

```powershell
./scripts/check.ps1 build
./scripts/run.ps1 -CheckBybit
```

`run.ps1` читает `.env` как данные, не исполняет его и не выводит значения. Команда проверит книги/инструменты, торговый тариф аккаунта, условия вывода BTC, P2P объявления и расчёт 0.01 BTC. Запросы только на чтение; POST `/v5/p2p/item/online` также является операцией чтения. Успех заканчивается реальной котировкой, ошибка — ненулевым exit code и названием этапа. API response messages не выводятся, чтобы исключить утечку credentials.

После успешной диагностики:

```powershell
./scripts/run.ps1
```

В Telegram отправьте `0.01 BTC`. Настройте payment method ID через `/settings <ID>`. Доступные IDs приходят в P2P API; отображение справочника названий банков пока не реализовано. Merchant conditions/KYC/банковские ограничения нужно проверять перед сделкой; котировка не резервирует объявление.

## Что проверяет adapter

- HMAC-SHA256 подпись точной query/body, новая timestamp/signature при HTTP retry.
- Обе официально показанные оболочки `retCode` и `ret_code`; ошибки прав доступа не маскируются.
- Объявления продавцов (`side=1`), RUB/USDT, доступное количество, лимиты, recent orders и completion rate, payment methods.
- До 10 страниц по 300 объявлений. Превышение, неполная/повторная пагинация возвращают ошибку; частичный список не выдаётся за весь рынок.
- По умолчанию отсутствующая P2P-комиссия исключает объявление. Опциональная настройка BYBIT_P2P_TAKER_FEE_FALLBACK=0 разрешает использовать явно заданную нулевую ставку для RUB/USDT, когда API возвращает пустые поля. Котировка всегда помечается IsDegraded с предупреждением. Явная ненулевая ставка не переопределяется. Точность token.scale обязательна.
- Реальный taker fee аккаунта, BTC withdrawal network/status/min/max/precision; процентный withdrawal пока отклоняется.
- Base/quote precision, minimum и maximum market quantity/value из instruments API.

`BYBIT_P2P_SOURCE=web` остаётся отдельной заглушкой. Автоматического обхода ограничений доступа и переключения на undocumented endpoints нет. Другие маршруты по-прежнему требуют интеграций; они не мешают работающему Bybit.

Контракты: [объявления](https://bybit-exchange.github.io/docs/p2p/ad/online-ad-list), [подпись](https://bybit-exchange.github.io/docs/v5/guide), [торговая комиссия](https://bybit-exchange.github.io/docs/v5/account/fee-rate), [вывод и сети](https://bybit-exchange.github.io/docs/v5/asset/coin-info), [инструменты](https://bybit-exchange.github.io/docs/v5/market/instrument).

2026-09-07 приватная проверка выполнена: торговая комиссия, условия вывода и P2P API доступны. После включения явно маркированной резервной P2P-комиссии получены 826 валидных объявлений и контрольная котировка 0.01 BTC. httptest покрывает полный маршрут, подпись, API errors, retries и опасные неполные данные. Публичный endpoint BTCUSDT проверен успешно.

