# T-0103 · TD: internal/domain/markdown.go превысил лимит 300 строк после T-0102

- Status (Статус): done
- Priority (Приоритет): low
- Created (Создан): 2026-10-27 11:02 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

internal/domain/markdown.go превысил лимит 300 строк после T-0102

## Details (Подробности)

После T-0102 (разбиение конкатенаций в b.WriteString по требованию gopls writestring) internal/domain/markdown.go вырос 299 → 323 строки, т.е. превысил проектный лимит 300 строк на файл (прецеденты: T-0005 store.go, T-0088 markdown_comments_legacy_test.go). Функция Render разрослась: 14 мест × +3 строки. Вариант решения: вынести Render (markdown.go:160-291) в отдельный файл internal/domain/render.go — это новый файл, поэтому в объём T-0102 не входило (границы тикета: только 3 файла + тест). См. план T-0102 (wiki c9dfeddc) и code review U3 (261ba4c7).

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-27 11:02 — тикет создан (erdmitry).
- 2026-10-28 20:37 — статус: open → wip · Начата декомпозиция internal/domain/markdown.go; полный pipeline T-0103. (erdmitry)
- 2026-10-28 23:04 — статус: wip → done · Verified: gofmt -l . clean; go vet ./... clean; go test ./... -count=1 = 572/6 PASS; staticcheck/gosec 0; independent whole-diff review 639f0bd9 APPROVE 🔴=0 🟡=0 🔵=0; Render byte-identical at internal/domain/markdown_render.go:12; markdown.go=195, markdown_render.go=133 ≤300. markdownlint: 7 pre-existing issues T-0112, not changed. (erdmitry)
