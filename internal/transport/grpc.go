// Package transport предоставляет транспортный уровень для взаимодействия с сервером.
// Реализует gRPC клиент с поддержкой TLS и аутентификации через JWT токены.
package transport

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"

	"github.com/prbllm/goph-keeper/api"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

// DialOptions содержит параметры для настройки gRPC соединения.
// Используется при создании нового клиента для конфигурации безопасности.
type DialOptions struct {
	// Insecure указывает на использование незащищённого соединения (без TLS).
	// Рекомендуется использовать только для разработки и тестирования.
	Insecure bool
	// CAPath определяет путь к файлу сертификата центра сертификации.
	// Используется для проверки сертификата сервера при защищённом соединении.
	CAPath string
}

// GRPCClient представляет обёртку над gRPC соединением с сервером.
// Содержит клиенты для всех доступных сервисов сервера.
type GRPCClient struct {
	// Conn — базовое gRPC соединение с сервером
	Conn *grpc.ClientConn

	// Auth — клиент сервиса аутентификации
	Auth api.AuthServiceClient
	// Vault — клиент сервиса управления хранилищем
	Vault api.VaultServiceClient
	// Sync — клиент сервиса синхронизации
	Sync api.SyncServiceClient
	// Blob — клиент сервиса хранения больших двоичных объектов
	Blob api.BlobServiceClient
}

// New создаёт новый gRPC клиент для взаимодействия с сервером.
// Принимает адрес сервера, токен доступа и параметры подключения.
// Настраивает соединение с поддержкой TLS или незащищённым режимом.
// Добавляет interceptor для автоматической аутентификации запросов.
// Возвращает указатель на клиент или ошибку инициализации.
func New(addr string, token string, opts DialOptions) (*GRPCClient, error) {
	dialOpts := []grpc.DialOption{grpc.WithUnaryInterceptor(AuthInterceptor(token))}

	if opts.Insecure {
		dialOpts = append(dialOpts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		certPool := x509.NewCertPool()

		ca, err := os.ReadFile(opts.CAPath)
		if err != nil {
			return nil, err
		}

		if !certPool.AppendCertsFromPEM(ca) {
			return nil, errors.New("invalid CA cert")
		}

		tlsConfig := &tls.Config{
			RootCAs:    certPool,
			MinVersion: tls.VersionTLS12,
		}

		creds := credentials.NewTLS(tlsConfig)

		dialOpts = append(dialOpts, grpc.WithTransportCredentials(creds))
	}

	conn, err := grpc.NewClient(
		addr,
		dialOpts...,
	)
	if err != nil {
		return nil, err
	}

	return &GRPCClient{
		Conn:  conn,
		Auth:  api.NewAuthServiceClient(conn),
		Vault: api.NewVaultServiceClient(conn),
		Sync:  api.NewSyncServiceClient(conn),
		Blob:  api.NewBlobServiceClient(conn),
	}, nil
}

func (c *GRPCClient) AuthClient() api.AuthServiceClient {
	return c.Auth
}

func (c *GRPCClient) VaultClient() api.VaultServiceClient {
	return c.Vault
}

func (c *GRPCClient) SyncClient() api.SyncServiceClient {
	return c.Sync
}

func (c *GRPCClient) BlobClient() api.BlobServiceClient {
	return c.Blob
}

// Close закрывает gRPC соединение с сервером.
// Освобождает все связанные ресурсы и завершает активные потоки.
// Должен вызываться после завершения работы с клиентом.
// Возвращает ошибку в случае неудачи закрытия соединения.
func (c *GRPCClient) Close() error {
	return c.Conn.Close()
}
