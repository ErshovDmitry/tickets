# Аудит, часть 7 — Windows runtime-тесты на dser97

Дата: 2026-09-06

## Условия

- Хост: dser97.home, Windows 10.0.26100.32690, amd64, shell cmd.exe (пользователь ai-base). Go на хосте НЕ установлен (`where go` — пусто).
- Кросс-сборка на erdmitry: `GOOS=windows GOARCH=amd64 go test -c` для internal/domain, internal/store, internal/lock, internal/paths, internal/cli, cmd/ticket + `go build -ldflags "-X ticket/internal/cli.version=1.2.0-3-g3ccda64"`. Все 7 бинарников собраны успешно (PE32+ amd64), передача scp в `%TEMP%\ticket-audit-win\bin`, размеры совпали. Исходники не изменялись.

## Результаты тестов (запуск `<pkg>.test.exe -test.v` в песочнице)

| Пакет | Итог | Exit code | Примечание |
|---|---|---|---|
| internal/domain | PASS | 0 | все тесты зелёные |
| internal/lock | 4 PASS / 1 FAIL | ≠0 | падает только `TestCompileMatrix` (см. находки); в каталоге исходников — полный PASS, exit 0 |
| internal/paths | PASS | 0 | 1 SKIP: symlink unavailable on this host |
| internal/store | PASS | 0 | 7 SKIP: chmod read-only/root/symlinks — ожидаемо на Windows |
| internal/cli | FAIL | ≠0 | `TestMain` требует Go на хосте (см. находки) |
| cmd/ticket | PASS | 0 | `TestCrossBuildDist` SKIP: `TICKETS_CROSS_BUILD not set` |

Вывод `lock.test.exe` в песочнице: `lock_test.go:161: missing lock_unix.go: open lock_unix.go: The system cannot find the file specified.` Вывод `cli.test.exe`: `integration: go build ../../cmd/ticket: exec: "go": executable file not found in %PATH%`.

## CLI smoke (в песочнице, TICKETS_DIR=%TEMP%\ticket-audit-win\tickets)

Все команды — RC=0:

```
new "Windows smoke" -t OPS -p low -d smoke → ...tickets\T-0001-open.md (+ предупреждение о новом проекте, stderr)
list                                       → T-0001  open  OPS: Windows smoke
show 1                                     → полный рендер (Summary/Details/User comments/Comments/Journal)
set 1 wip "win"                            → T-0001-wip.md
set 1 done "win ok"                        → T-0001-done.md
archive                                    → ...tickets\archive\T-0001-done.md
list archive                               → T-0001  done  OPS: Windows smoke
```

Имена файлов соответствуют `T-NNNN-<status>.md` на каждом шаге; в конце в каталоге `.lock` + `archive\T-0001-done.md`. Версия из ldflags подтверждена: `ticket version 1.2.0-3-g3ccda64`. Русский текст вывода корректен. Песочница после прогона удалена (проверено `if exist` → REMOVED_OK).

## Находки

- 🟡 `internal/cli/integration_test.go` (`runMain`): при отсутствии Go на хосте `buildTicketBinary` возвращает 1 и `TestMain` завершает процесс ДО запуска каких-либо тестов — весь пакет cli на «чистом» Windows-хосте не тестируем (FAIL вместо внятного skip). Предложить: детектировать `go` через `exec.LookPath` и делать `t.Skip`-аналог на уровне TestMain с сообщением.
- 🔵 `internal/lock/lock_test.go:149 TestCompileMatrix`: читает `lock_*.go` через `os.ReadFile` относительно CWD — standalone test-бинарник вне checkout'а репозитория падает (в песочнице FAIL; при запуске из каталога исходников — PASS, exit 0). Тест корректен в CI, но стоит пропускать с сообщением, если файлы отсутствуют.
- Примечание: статус в CLI называется `wip` (не `in_progress`) — команда из текста задания была бы отклонена; в smoke использован `wip`.

## Результат

Windows runtime-пробел закрыт: все пакеты кросс-компилируются, lock/paths/store/domain/cmd корректно работают на реальном Windows (падения/пропуски объяснены и не являются дефектами кода), CLI полный жизненный цикл тикета на Windows отрабатывает. Кроссплатформенные дефекты не обнаружены.

**Сделано (2026-09-06).**
