# T-0108 · TD: unusedparams: неиспользуемые параметры project/who в cmdShow и cmdSet

- Status (Статус): done
- Priority (Приоритет): low
- Created (Создан): 2026-10-27 14:07 · by (кем): erdmitry
- Project (Проект): tickets

## Summary (Кратко)

unusedparams: неиспользуемые параметры project/who в cmdShow и cmdSet

## Details (Подробности)

pi-lens information (не warning): internal/cli/cmd_show.go:18:46 (who), 18:51 (project), internal/cli/cmd_set.go:19:50 (project). Эмпирическая проверка 2026-10-27: в cmd_show.go оба параметра не используются; в cmd_set.go параметр who ИСПОЛЬЗУЕТСЯ (строка 58: st.SetStatus(n, next, who, comment)), а project — нет. Фикс минимальный и без изменения контракта диспетчеризации: оставить сигнатуру, заменить имена на _ в нужных позициях (cmdShow: who, project -> _; cmdSet: project -> _). Проверено независимыми ревьюерами kimi-k3.1-pro (замечание про who в cmdSet у линзы корректно, а в моём устном пересказе было ошибочно сказано «оба»).

## User comments (Комментарии от пользователя)

## Comments (Комментарии)

## Journal (Журнал)

- 2026-10-27 14:07 — тикет создан (erdmitry).
- 2026-10-28 20:28 — статус: open → done · Проверено: targeted go test ./internal/cli → PASS; go test ./... → PASS (6 packages); gofmt/go vet/staticcheck/gosec → exit 0; lens unusedparams=0; независимый V1 APPROVE 🔴=0 🟡=0. internal/cli/cmd_show.go:18 и cmd_set.go:19; who сохранён cmd_set.go:58. markdownlint: 7 предсуществующих ошибок → T-0112. (erdmitry)
