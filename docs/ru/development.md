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
