# T-0117 · TD: WikiDB: отсутствуют contracts для четырёх использованных MCP tools

- Status (Статус): open
- Priority (Приоритет): low
- Created (Создан): 2026-10-28 22:58 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

WikiDB: отсутствуют contracts для четырёх использованных MCP tools

## Details (Подробности)

Попутная находка T-0103. resources/read config://tools/model_limits, code_map_query, code_graph_impact, get_links вернул Resource not found; get_links эмпирически принимает id, не page_id. Нужно добавить/актуализировать canonical contracts отдельной инфраструктурной задачей; поведение волны не блокирует.

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-28 22:58 — тикет создан (erdmitry).
