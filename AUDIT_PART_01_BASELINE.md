# Аудит, часть 1 — baseline и структура

Дата: 2026-09-06

## Проверки

### go version
- Код возврата: `0`
- stdout:
```
go version go1.26.7-X:nodwarf5 linux/amd64

```
- stderr:
```

```

### gofmt
- Код возврата: `0`
- stdout:
```

```
- stderr:
```

```

### go vet
- Код возврата: `0`
- stdout:
```

```
- stderr:
```

```

### go test
- Код возврата: `0`
- stdout:
```
ok  	ticket/cmd/ticket	(cached)
ok  	ticket/internal/cli	(cached)
ok  	ticket/internal/domain	(cached)
ok  	ticket/internal/lock	(cached)
ok  	ticket/internal/paths	(cached)
ok  	ticket/internal/store	(cached)

```
- stderr:
```

```

### go build
- Код возврата: `0`
- stdout:
```

```
- stderr:
```

```

## Итог

Baseline-проверки выполнены. Подробных замечаний по результатам этих команд не выявлено, если код возврата равен 0. Следующая часть: аудит `internal/domain`.
