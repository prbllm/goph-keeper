package transport

import "github.com/prbllm/goph-keeper/api"

// Client — интерфейс для транспортного клиента
type Client interface {
	AuthClient() api.AuthServiceClient
	VaultClient() api.VaultServiceClient
	SyncClient() api.SyncServiceClient
	BlobClient() api.BlobServiceClient
	Close() error
}
