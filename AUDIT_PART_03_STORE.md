# Аудит, часть 3 — store и файловая надёжность

Дата: 2026-09-06

## Проверки
- `go test -race ./internal/store` — успешно.
- Проверены scan/safeopen/find/create/setStatus/archive.

## Замечания
- 🟡 `internal/store/archive.go: ArchiveClosed` игнорирует ошибки `s.scan()` и warnings. При проблемах чтения каталога возможна частичная обработка без явного сообщения. Рекомендация: обрабатывать `dirErr` и warnings, сохраняя уже перемещённые пути.
- 🟡 `internal/store/store.go: Create` мутирует переданный `*Ticket` (как минимум `Status` и `Number`). Рекомендация: либо работать с копией, либо явно закрепить mutation contract в API/документации.

## Положительные результаты
Symlink/special-file rejection, TOCTOU identity check, authoritative filename status, no-replace commit, rollback paths, archive numbering и race-тесты присутствуют.

**Сделано (2026-09-06).**
