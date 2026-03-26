package transport

import gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"

// Client — интерфейс для транспортного клиента
type Client interface {
	AuthClient() gophkeeperv1.AuthServiceClient
	VaultClient() gophkeeperv1.VaultServiceClient
	SyncClient() gophkeeperv1.SyncServiceClient
	BlobClient() gophkeeperv1.BlobServiceClient
	Close() error
}
