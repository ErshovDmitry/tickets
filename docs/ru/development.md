# Разработка

Сборка `ticket` из исходников, кросс-компиляция под все поддерживаемые цели и проверки, которые обязаны пройти перед коммитом.

## Требования

- Go >= 1.26.
- Единственная внешняя зависимость — `golang.org/x/sys`, нужна для блокировки файлов на Windows.
- Сборка с `CGO_ENABLED=0` — на выходе статический бинарник с нулём runtime-зависимостей.

## Сборка из исходников

Версия бинаря — из git-тега (фолбэк `dev`); задайте переменную один раз перед сборкой:

```bash
VER=$(git describe --tags --always 2>/dev/null | sed 's/^v//'); VER=${VER:-dev}
```

Текущая платформа:

```bash
CGO_ENABLED=0 go build -ldflags "-X ticket/internal/cli.version=$VER" -o ~/bin/ticket ./cmd/ticket
```

Установка в `PATH` и подготовка проекта: [installation.md](installation.md).

### Кросс-компиляция

Везде `CGO_ENABLED=0`, формат `GOOS=... GOARCH=... go build -o dist/<имя> ./cmd/ticket`:

| Цель | Команда |
|------|---------|
| linux/amd64 | `CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-amd64 ./cmd/ticket` |
| linux/arm64 | `CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-linux-arm64 ./cmd/ticket` |
| windows/amd64 | `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket.exe ./cmd/ticket` |
| windows/arm64 | `CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-windows-arm64.exe ./cmd/ticket` |
| darwin/amd64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-amd64 ./cmd/ticket` |
| darwin/arm64 | `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags "-X ticket/internal/cli.version=$VER" -o dist/ticket-darwin-arm64 ./cmd/ticket` |

> ⚠️ На ФС без поддержки hardlinks (FAT/exFAT, частый случай на съёмных носителях под Windows) команды `set` и `archive` падают с ошибкой `link`. Это fail-safe: данные не теряются. Держите `tickets/` на NTFS или POSIX-ФС с поддержкой hardlinks.

## Гигиена

Обязательно чисто перед коммитом и зелёно перед отметкой done:

```bash
gofmt -l . && go vet ./...      # обязательно чисто перед коммитом
go test ./...                   # обязательно зелёное перед отметкой done
```

## Markdown lint

Документация проверяется линтером [markdownlint-cli2](https://github.com/DavidAnson/markdownlint-cli2); конфигурация — в `.markdownlint-cli2.jsonc`.

Что проверяется: структура и гигиена Markdown — пустые строки вокруг заголовков (MD022) и блоков кода (MD031), списки в окружении пустых строк (MD032), отсутствие inline-HTML там, где есть Markdown-конструкция (MD033), отсутствие «голых» URL (MD034), языковой тег у каждого fenced-блока (MD040).

Отключённые правила и почему:

- **MD013** (длина строки) — проза в `docs/` пишется длинными строками (до 420 символов); перенос означал бы массовый рефлоу всех документов и выходит за рамки этой задачи.
- **MD060** (стиль колонок таблицы) — 36 нарушений в 7 таблицах, где смешаны компактный и выровненный стили колонок; приведение таблиц к единому виду — отдельная задача.

Скоуп: `tickets/archive/**` исключён по решению пользователя (2026-10-26, пункт 2b); активные `tickets/*.md` линтуются. Также исключены: `docs_api/**`, `AGENTS.md`, `AGENTS_ARCHITECTURE.md`, `DEPLOY.md`, `node_modules/**`, `dist/**`, `internal/cli/testdata/**` — побайтовые golden-фикстуры генератора исключены из lint-скоупа, их формат проверяет golden-тест.

Локальный запуск — версия CLI зафиксирована в .github/workflows/ci.yml:

```bash
markdownlint-cli2 --no-globs $(git ls-files '*.md')
```

Обычный запуск `markdownlint-cli2` (без `--no-globs`) дополнительно проверяет активные тикеты `tickets/*.md` (каталог `tickets/archive/**` исключён конфигом), а команда с `--no-globs` выше проверяет только трекаемое содержимое репозитория.

## Windows smoke

Кросс-компилируйте `dist/ticket.exe` (windows/amd64, таблица выше) и прогоните `scripts/smoke-windows.ps1` на Windows-хосте (вручную по SSH):

```powershell
powershell -ExecutionPolicy Bypass -File scripts\smoke-windows.ps1
```

Скрипт работает целиком внутри временной песочницы — репозиторий и данные пользователя не затрагиваются. Код выхода 0 — все шаги прошли, 1 — хотя бы один упал. Семь шагов:

1. `new` — создаёт тикет и печатает путь к файлу;
2. `list` — показывает тикет;
3. `show` — печатает тело тикета;
4. `set` — переименовывает файл, обновляет строку статуса и журнал;
5. глобальный флаг `-C` / `--tickets-dir` из чужой рабочей директории;
6. override через переменную окружения `TICKETS_DIR`;
7. параллельный `new` ×5 — пять уникальных последовательных номеров через OS-лок.
