# T-0111 · TD: TD: docs_api/add-language.md описывает старую панику usage

- Status (Статус): done
- Priority (Приоритет): low
- Created (Создан): 2026-10-27 22:25 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

TD: docs_api/add-language.md описывает старую панику usage

## Details (Подробности)

После T-0107 docs_api/add-language.md:50 продолжает требовать fast-stop panic для отсутствующего templates/usage.<lang>.txt. Обновить документацию отдельной задачей: runtime EN fallback + ошибка/stderr/nonzero; сам файл вне scope T-0107.

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-27 22:25 — тикет создан (erdmitry).
- 2026-10-28 17:40 — статус: open → wip · Начата doc-only актуализация: эмпирическая проверка fallback и правка docs_api/add-language.md делегированы. (erdmitry)
- 2026-10-28 18:17 — статус: wip → done · Проверено: /tmp smoke (known de, no usage.de.txt) → EN stdout + 1 stderr line + exit 1; go test ./... → 572 passed; go vet ./... → clean; fresh atom reviews APPROVE 🔴=0 🟡=0 🔵=0; docs_api/add-language.md:56-86,116-155. (erdmitry)
