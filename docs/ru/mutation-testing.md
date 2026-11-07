# Mutation-тестирование

## Установка

Пин версии — `scripts/mutation.sh`:

```bash
GOBIN=./.cache/gobin go install github.com/go-gremlins/gremlins/cmd/gremlins@v0.6.0
```

`GOBIN` задан явно → бинарь в `./.cache/gobin`; при заданном `$GOBIN` — туда. Скрипт ищет `$GOBIN/gremlins` или `./.cache/gobin/gremlins`. Пути `/tmp` не используются.

Проверено: Go go1.26.10, раннер v0.6.0.

Почему gremlins: `go-mutesting` паникует в `go/types` на go1.26.10, `ooze` v0.2.0 — ложные killed (`go:embed`+symlink, T-0136).

## ⚠️ Предупреждение

Мутации идут В РАБОЧЕМ дереве. Грязное дерево и параллельный запуск запрещены: скрипт откажется (лок `./.cache/mutation.lock`).

## Запуск

```bash
./scripts/mutation.sh
```

Коды: 0 — успех, 1 — ошибка, 2 — неверные аргументы; INT/TERM — 130/143. Дефолты: скоуп `internal/lock`, `--workers 2`, `--timeout-coefficient 20`.
