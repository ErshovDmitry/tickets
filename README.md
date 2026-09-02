# ticket

## Что это

Кроссплатформенная (Windows / Linux / macOS) CLI-система тикетов на Go.

- Тикеты — обычные Markdown-файлы `T-NNNN-<status>.md` в каталоге `tickets/` проекта. Единственный источник правды — файлы: читаются человеком без инструментов (`cat`, редактор, `git diff`), база данных не нужна.
- Один статический бинарник `ticket` (`ticket.exe` на Windows), собирается с `CGO_ENABLED=0`, ноль runtime-зависимостей.
- Кроссплатформенный аналог bash-версии [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md).

## Установка

Готовый бинарник кладётся в `<проект>/tickets/bin/` — оттуда он сам находит каталог тикетов и работает из любой текущей директории (резолюция: `$TICKETS_DIR` → восходящий поиск `tickets/` от текущей директории → каталог относительно бинаря).

### Сборка из исходников

Требуется Go >= 1.26 (единственная внешняя зависимость — `golang.org/x/sys`, нужна для блокировки файлов на Windows).

Версия бинаря — из git-тега (фолбэк `dev`); задайте переменную один раз перед сборкой:

```bash
VER=$(git describe --tags --always 2>/dev/null | sed 's/^v//'); VER=${VER:-dev}
```

Текущая платформа:

```bash
go build -ldflags "-X ticket/internal/cli.version=$VER" -o tickets/bin/ticket ./cmd/ticket
```

Кросс-компиляция (везде `CGO_ENABLED=0`, формат `GOOS=... GOARCH=... go build -o dist/<имя> ./cmd/ticket`):

| Цель | Команда |
|------|---------|
| linux/amd64 | `GOOS=linux GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-amd64 ./cmd/ticket` |
| linux/arm64 | `GOOS=linux GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-arm64 ./cmd/ticket` |
| windows/amd64 | `GOOS=windows GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket.exe ./cmd/ticket` |
| windows/arm64 | `GOOS=windows GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-windows-arm64.exe ./cmd/ticket` |
| darwin/amd64 | `GOOS=darwin GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-amd64 ./cmd/ticket` |
| darwin/arm64 | `GOOS=darwin GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-arm64 ./cmd/ticket` |

> ⚠️ На ФС без поддержки hardlinks (FAT/exFAT на съёмных носителях) команды `set` и `archive` падают с ошибкой `link` — это fail-safe, данные не теряются; держите `tickets/` на NTFS или POSIX-ФС, подробности в [DEPLOY.md](DEPLOY.md).

Если проект под git — решите, коммитить `tickets/` вместе с проектом или добавить его в `.gitignore`.

## Быстрый старт

```bash
# создать тикет (статус open), печатает путь к файлу
ticket new "Поломка экспорта" -t BUG -p high -d "Падает на больших файлах" -w ivan

# список тикетов: по умолчанию active (= open + wip)
ticket list
ticket list all        # включая done/closed
ticket list archive    # только архив

# показать тикет (ищет и в архиве)
ticket show 7

# сменить статус: open → wip → done / closed
ticket set 7 wip "взял в работу"
ticket set 7 done "починено, тесты зелёные"

# перенести закрытые тикеты (done/closed) в tickets/archive/
ticket archive         # все закрытые
ticket archive 7       # один указанный

# версия бинарника (dev, если собран без -ldflags)
ticket version
```

## Конфигурация (env)

| Переменная | Назначение |
|------------|------------|
| `TICKETS_DIR` | явный override каталога тикетов |
| `TICKET_WHO` | автор тикета; цепочка `TICKET_WHO → USER → USERNAME → agent` |
| `TICKET_LANG` | язык справки `ticket help`; цепочка `TICKET_LANG → LC_ALL → LANG`, по умолчанию **RU** (EN — opt-in: `TICKET_LANG=en`) |

## Формат тикета

- Имя файла: `T-NNNN-<status>.md`; номер выдаётся атомарно через OS-локи и не переиспользуется.
- Статус хранится в имени файла. Вручную файлы не переименовывать: смену статуса делает `ticket set` (сам переименует файл и допишет журнал тикета).
- Жизненный цикл: `open` (новый) → `wip` (в работе) → `done` (исправлено) / `closed` (отклонён/дубликат).
- Типы: `BUG` — поломка; `OPS` — инцидент/обслуживание; `TD` — техдолг; `ENH` — улучшение.
- Архив: `ticket archive` переносит закрытые тикеты в `tickets/archive/`; `list archive` смотрит архив; `show` и `set` работают с архивом, перевод архивного тикета в `open`/`wip` возвращает его в работу.
- Секреты (пароли, токены, ключи) в тикеты НЕ писать.

## Ссылки

- bash-оригинал: [init-tickets.md](https://gitlab.com/ai-dmitry/promts/-/blob/main/init-tickets.md)
- `AGENTS_ARCHITECTURE.md` — архитектура (локальный файл, в репозиторий не публикуется)
- `DEPLOY.md` — сборка, требования к файловой системе

## Статус

**v1 done.**
