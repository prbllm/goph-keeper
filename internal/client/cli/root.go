// Package cli предоставляет корневую команду и глобальные флаги CLI.
// Инициализирует основную структуру командного интерфейса.
package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// rootCmd — корневая команда CLI приложения.
// Предоставляет базовый интерфейс и объединяет все подкоманды.
var rootCmd = &cobra.Command{
	Use:   "goph-keeper",
	Short: "Secure password manager CLI",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	SilenceUsage: true,
}

// Execute запускает выполнение командного интерфейса.
// Парсит аргументы командной строки и выполняет соответствующую команду.
// Возвращает ошибку в случае неудачи.
func Execute() error {
	return rootCmd.Execute()
}

// init инициализирует корневую команду и регистрирует все подкоманды.
// Настраивает глобальные флаги и пути по умолчанию.
func init() {
	dataDirPath, _ := os.UserHomeDir()
	dataDirPath = filepath.Join(dataDirPath, ".gophkeeper")
	rootCmd.PersistentFlags().StringP("server", "s", "localhost:50051", "gRPC server address")
	rootCmd.PersistentFlags().StringP("data-dir", "d", dataDirPath, "directory for storing client data")
	rootCmd.PersistentFlags().Bool("insecure", false, "disable TLS")
	rootCmd.PersistentFlags().MarkHidden("insecure")
	rootCmd.PersistentFlags().String("tls-ca", "cert.pem", "path to self-signed CA certificate")
	// auth
	authGroup := &cobra.Group{ID: "auth", Title: "Authentication Commands:"}
	rootCmd.AddGroup(authGroup)
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(loginCmd)
	// vault
	vaultGroup := &cobra.Group{ID: "vault", Title: "Vault Commands:"}
	rootCmd.AddGroup(vaultGroup)
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	fileGroup := &cobra.Group{ID: "file", Title: "File Commands:"}
	rootCmd.AddGroup(fileGroup)
	rootCmd.AddCommand(uploadCmd)
	rootCmd.AddCommand(downloadCmd)
	// sync
	syncGroup := &cobra.Group{ID: "sync", Title: "Sync Commands:"}
	rootCmd.AddGroup(syncGroup)
	rootCmd.AddCommand(syncCmd)
	// version
	rootCmd.AddCommand(versionCmd)
}
