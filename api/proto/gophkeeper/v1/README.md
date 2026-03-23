# gophkeeper.v1 proto

Общий protobuf-контракт для сервера и клиента.

## Генерация Go-кода

Из корня репозитория:

```bash
go generate ./api/proto/gophkeeper/v1
```

Либо вручную из этой директории:

```bash
protoc --proto_path=. \
  --go_out=. --go_opt=paths=source_relative \
  --go-grpc_out=. --go-grpc_opt=paths=source_relative \
  gophkeeper.proto
```

Сгенерированные файлы `gophkeeper.pb.go` и `gophkeeper_grpc.pb.go` хранятся рядом с `.proto` и доступны для импорта и сервером, и клиентом:

`github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1`
