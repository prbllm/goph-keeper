# CLI-клиент (`internal/client`)

Реализация клиента. Серверный код не импортирует этот пакет; контракт — только через `api/proto`.

## Содержимое

- Точка входа CLI: [`cmd/client`](../../cmd/client/README.md)  
- Транспорт gRPC (TLS, опционально insecure, перехватчик JWT): [`transport`](transport/README.md)  

## Запуск

Параметры подключения задаются флагами CLI (адрес, путь к CA, TLS и т.д.) — см. `cmd/client` и пакет `cli`.

При работе с учебным сервером по TLS клиент должен доверять CA (например `certs/ca.crt` после `scripts/generate_certs.sh`).

## Лимиты gRPC

Размеры сообщений на стороне клиента (`MaxCallSendMsgSize` / `MaxCallRecvMsgSize`) должны быть **не меньше** значения `GOPHKEEPER_GRPC_MAX_MESSAGE_BYTES` на сервере; иначе на крупных ответах возможен `RESOURCE_EXHAUSTED`. Подробнее — в [.env.example](../../.env.example) и [internal/server/config/README.md](../server/config/README.md).
