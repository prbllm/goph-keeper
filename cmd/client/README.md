# Точка входа CLI (`cmd/client`)

`main` вызывает [`cli.Execute()`](../../internal/client/cli/root.go) из пакета [`internal/client/cli`](../../internal/client/cli/root.go). Команды: auth, vault, файлы, sync — см. `goph-keeper --help`.

## Запуск

Из корня репозитория:

```bash
go run ./cmd/client --help
```

TLS, адрес сервера и каталог данных — см. [корневой README](../../README.md) (раздел «Клиент») и [internal/client/README.md](../../internal/client/README.md).
