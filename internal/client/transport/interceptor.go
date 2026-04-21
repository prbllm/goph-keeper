// Package transport предоставляет интерцепторы для gRPC клиента.
// Реализует автоматическую аутентификацию запросов через заголовки.
package transport

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// AuthInterceptor создаёт интерцептор для добавления JWT токена в запросы.
// Принимает токен доступа и возвращает функцию-интерцептор.
// Добавляет заголовок "authorization" с префиксом "Bearer " к каждому исходящему запросу.
// Если токен пустой, запрос отправляется без аутентификации.
func AuthInterceptor(token string) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req any,
		reply any,
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		opts ...grpc.CallOption,
	) error {

		if token != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		}

		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

// StreamAuthInterceptor добавляет тот же токен Bearer в клиентские потоковые RPC (например, загрузка/скачивание Blob).
func StreamAuthInterceptor(token string) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if token != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}
