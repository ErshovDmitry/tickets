# T-0106 · TD: ветка domain.ErrJournalNewline в cmdSet не покрыта тестом CLI

- Status (Статус): done
- Priority (Приоритет): low
- Created (Создан): 2026-10-27 13:37 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

ветка domain.ErrJournalNewline в cmdSet не покрыта тестом CLI

## Details (Подробности)

Найдено при code review T-0105 (internal/cli/cmd_set.go:64-67). go test ./internal/cli -coverprofile: `cmd_set.go:64.69,67.4 2 0` — счётчик 0, ветка не исполняется ни одним тестом. Ветка существовала до волны, регрессии не вносит: CLI заранее валидирует CR/LF в comment/who (cmd_set.go:44-47, cli.go:162), а store-уровень ErrInvalidJournalInput покрыт (internal/store/setstatus_injection_test.go:30,65,91,121). Пробел: если эта предвалидация будет ослаблена/убрana, локализованное сообщение domain.ErrJournalNewline(lang) перестанет проверяться. Нужен тест cmdSet с подставленным store, возвращающим *store.ErrInvalidJournalInput, с проверкой текста и кода возврата 1.

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-27 13:37 — тикет создан (erdmitry).
- 2026-10-28 15:51 — статус: open → wip · Начата test-only волна T-0106; план и независимые ревью по gate pipeline. (erdmitry)
- 2026-10-28 17:04 — статус: wip → done · go test ./internal/cli -run '^TestCmdSetStoreJournalError$' -count=1 -v: PASS RU/EN; gofmt -l . пусто, go vet ./... и go test ./... PASS; coverage cmd_set.go:64.69,67.4=2; тест internal/cli/cmd_set_ctl_test.go:22. (erdmitry)
