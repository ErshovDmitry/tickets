# T-0115 · TD: CI: комментарий Staticcheck всё ещё ссылается на закрытые находки T-0102

- Status (Статус): open
- Priority (Приоритет): low
- Created (Создан): 2026-10-28 21:11 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

CI: комментарий Staticcheck всё ещё ссылается на закрытые находки T-0102

## Details (Подробности)

Попутная находка plan-review T-0103. .github/workflows/ci.yml:68-71 утверждает, что существуют ровно два known findings SA4006/SA6006 и они owned by T-0102. Фактически T-0102 done, /home/erdmitry/go/bin/staticcheck ./... exit 0 без вывода 2026-10-28. Обновить комментарий отдельно, не в объёме T-0103.

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-28 21:11 — тикет создан (erdmitry).
