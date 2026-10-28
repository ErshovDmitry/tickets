# T-0114 · TD: AGENTS_ARCHITECTURE: дерево internal/domain не отражает текущие markdown-файлы

- Status (Статус): open
- Priority (Приоритет): low
- Created (Создан): 2026-10-28 20:57 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

AGENTS_ARCHITECTURE: дерево internal/domain не отражает текущие markdown-файлы

## Details (Подробности)

Попутная находка планирования T-0103. AGENTS_ARCHITECTURE.md:30-36 перечисляет markdown.go как единый рендер+парсер и не показывает уже существующие markdown_parse.go/journal.go; после T-0103 добавится markdown_render.go. Обновление архитектурного router требует отдельной Decision-страницы и не входит в границы T-0103.

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-28 20:57 — тикет создан (erdmitry).
