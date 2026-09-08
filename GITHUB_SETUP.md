# GitHub и деплой kurskonverter

Репозиторий: https://github.com/tishin-serg/kurskonverter
Пример организации запуска: https://github.com/tishin-serg/magnetto/blob/master/.github/workflows/ci-cd.yml

## Как запускается

- Рабочие ветки `ST/*`: CI без публикации и деплоя.
- PR в `main`: проверки Go, unit/race, staticcheck, govulncheck, сценарии rollback, Docker build.
- Push/merge в `main`: те же проверки → Docker image в GHCR → SSH-деплой точного digest.
- `CI/CD → Run workflow → main`: повторный запуск после заполнения секретов.
- Теги `vX.Y.Z`: дополнительный Release с Linux binaries, checksums и образом; также вызывает Deploy.

Если секретов нет, деплой пропускается с явным пояснением в summary. Успешная сборка при пропущенном деплое не означает, что бот уже перенесён на VPS. PR и ручной CI/CD с рабочей ветки ничего не разворачивают.

## Секреты

Заполнить Settings → Secrets and variables → Actions → Repository secrets, как в magnetto. Также допустимы секреты Environment `production` (они имеют приоритет).

| Secret | Значение |
|---|---|
| `VDS_HOST` | DNS или IPv4 VPS |
| `VDS_USER` | SSH-пользователь с доступом к Docker и /opt/kurskonverter |
| `VDS_PORT` | SSH-порт, необязательно, по умолчанию 22 |
| `VDS_SSH_PRIVATE_KEY` | Выделенный приватный ключ SSH |
| `VDS_KNOWN_HOSTS` | Проверенная запись host key VPS |

Имена первых четырёх совпадают с magnetto. Значения секретов не копировались из другого репозитория. Ключ хоста сверяется по доверенному каналу; не подставлять непроверенный результат ssh-keyscan.

GHCR использует встроенный `GITHUB_TOKEN`. Постоянный registry token не требуется. Если package private, проверить доступ Actions этого репозитория к пакету.

## Первый запуск на сервере

Нужны Docker Engine, Compose v2, Bash и flock. Сборки выполняются на GitHub. Пользователь `VDS_USER` должен иметь возможность выполнять Docker-команды и записывать в каталог проекта; workflow не вызывает sudo и не использует `/usr/local/sbin/magnetto-deploy`.

1. Создать `/opt/kurskonverter`, доступный пользователю деплоя.
2. Установить `deploy/compose.yaml` из этого репозитория как `/opt/kurskonverter/compose.yaml`.
3. Подготовить `/opt/kurskonverter/.env` с реальными ключами приложения, права 600. Пример — `.env.example`. API-ключи бота/бирж не входят в образ и не публикуются в GitHub.
4. При переносе сохранить SQLite из локального экземпляра. Контейнер использует volume `kurskonverter_bot-data` и файл `/data/bot.db`; владелец в контейнере UID/GID 10001. Миграции выполняются при запуске.
5. Перед запуском VPS остановить локальный polling с тем же Telegram-токеном. Проверить ограничения IP у биржевых API-ключей для IP нового сервера.
6. После заполнения секретов запустить CI/CD на `main` и проверить job Deploy.

Compose project: `kurskonverter`. HTTP привязан к `127.0.0.1:18089`, внутри контейнера 8080. Конфигурация и сервисы magnetto в `/opt/torrentbot` не изменяются. Серверный compose установлен отдельно: при его изменении в репозитории обновить файл на сервере до следующего деплоя.

## Проверки и откат

Деплой требует Docker health и `bot healthcheck ready`. При неудаче возвращается предыдущий digest. При первом неудачном запуске контейнер останавливается, так как предыдущего образа ещё нет. SQLite автоматически назад не откатывается; миграции должны быть совместимы с предыдущей версией.

Точный digest доступен в artifact `deployment-image/image.txt` и summary CI/CD. Ручной откат: Actions → Deploy → Run workflow → digest предыдущего успешного образа и его версия.

Дальнейшая работа: `ST/*` → PR → проверки → merge в `main` → автоматический деплой. После первого CI настроить required status checks для main; возможность защиты ветки зависит от плана и доступа к репозиторию.
