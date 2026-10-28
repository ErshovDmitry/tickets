# T-0113 · TD: TD: шесть файлов вне T-0103 превышают проектный лимит 300 строк

- Status (Статус): open
- Priority (Приоритет): low
- Created (Создан): 2026-10-28 20:46 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

TD: шесть файлов вне T-0103 превышают проектный лимит 300 строк

## Details (Подробности)

Попутная находка recon T-0103, не исправлять в объёме T-0103. Замерено wc -l 2026-10-28: internal/domain/i18n.go=1097; internal/domain/i18n_init_test.go=649; internal/cli/run_order_test.go=625; internal/domain/markdown_test.go=351; internal/cli/cli_test.go=326; internal/cli/cmd_new_test.go=304. Требуется отдельная декомпозиция по логическим границам без изменения поведения.

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-28 20:46 — тикет создан (erdmitry).
