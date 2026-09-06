# План аудита проекта tickets

Статус: аудит выполняется, изменения исходников не вносятся. Все замечания дублируются в этом файле.

## Этапы и результаты

- [x] Часть 1 — baseline — **Сделано (2026-09-06)** → `AUDIT_PART_01_BASELINE.md`
- [x] Часть 2 — domain — **Сделано (2026-09-06)** → `AUDIT_PART_02_DOMAIN.md`
  - 🔴 Title допускает `\r`/`\n`; после `set` многострочный заголовок частично теряется. Отклонять переводы строк и добавить regression-тест.
  - 🟡 Journal parsing останавливается на первой нераспознанной строке; закрепить контракт.
- [x] Часть 3 — store — **Сделано (2026-09-06)** → `AUDIT_PART_03_STORE.md`
  - 🟡 `ArchiveClosed` игнорирует ошибки сканирования; обрабатывать `dirErr`/warnings.
  - 🟡 `Create` мутирует входной `*Ticket`; уточнить или изменить контракт.
- [x] Часть 4 — lock/paths — **Сделано (2026-09-06)** → `AUDIT_PART_04_LOCK_PATHS.md`
  - 🔵 Windows runtime-тесты требуют Windows host; cross-build проверен. Закрыто в части 7.
- [x] Часть 5 — CLI и UX — **Сделано (2026-09-06)** → `AUDIT_PART_05_CLI.md`
  - 🟡 `cmd_list.go`: лишние positional args игнорируются; отклонять их.
  - 🟡 warnings при list не меняют exit code; задокументировать policy.
- [x] Часть 6 — тесты и документация — **Сделано (2026-09-06)** → `AUDIT_PART_06_TESTS_DOCS.md` (файл существовал до отметки; расхождение плана устранено)
- [x] Часть 7 — Windows runtime на dser97 — **Сделано (2026-09-06)** → `AUDIT_PART_07_WINDOWS.md`
  - 🟡 `internal/cli`: без Go на хосте TestMain завершает весь пакет FAIL до старта тестов; нужен graceful skip.
  - 🔵 `internal/lock` TestCompileMatrix требует запуска из каталога исходников (читает `lock_*.go` через os.ReadFile).
- [x] Итоговый отчёт — **Сделано (2026-09-06)** → `AUDIT_FINAL.md`

## Общие нерешённые замечания

- [ ] 🔴 Обработать многострочные title.
- [ ] 🟡 Обработать ошибки `ArchiveClosed`.
- [ ] 🟡 Уточнить mutation contract `Create`.
- [ ] 🟡 cli-тесты неработоспособны на хосте без Go (TestMain build step).
