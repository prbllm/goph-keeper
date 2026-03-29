# gRPC-слой (`internal/server/grpcserver`)

Пакет собирает **только TLS** gRPC-сервер: регистрирует `Auth`, `Vault`, `Blob`, `Sync`, `Device`, health и reflection (если включено в коде), подключает цепочку interceptors.

## Interceptors

- **Logging** — метод, длительность, статус (единый формат с потоковыми вызовами).  
- **Recovery** — перехват паник.  
- **Auth** — разбор JWT, прокидывание `user_id` / `device_id` в `context` для закрытых RPC.  
- Для потоков blob — проверка контекста аутентификации на стороне handler.

## Handlers

Соответствуют сервисам в [`api/proto/gophkeeper/v1`](../../../api/proto/gophkeeper/v1/gophkeeper.proto): аутентификация, vault CRUD, загрузка/скачивание blob, sync push/pull, список устройств и отзыв.

## См. также

- [internal/server/README.md](../README.md)  
- [docs/server](../../docs/server/README.md)  
