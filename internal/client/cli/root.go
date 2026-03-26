// Package cli предоставляет корневую команду и глобальные флаги CLI.
// Инициализирует основную структуру командного интерфейса.
package cli

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// serverAddr — глобальная переменная адреса gRPC сервера.
// Значение по умолчанию: "localhost:50051".
var serverAddr = "localhost:50051"

// insecure — глобальная переменная флага отключения TLS.
// Если true, используется незащищённое соединение.
var insecure = false

// tlsCA — глобальная переменная пути к файлу сертификата ЦС.
// Используется для проверки сертификата сервера.
var tlsCA = "cert.pem"

// dataDirPath — глобальная переменная пути к директории данных клиента.
// По умолчанию: ~/.gophkeeper
var dataDirPath, _ = os.UserHomeDir()

// rootCmd — корневая команда CLI приложения.
// Предоставляет базовый интерфейс и объединяет все подкоманды.
var rootCmd = &cobra.Command{
	Use: "goph-keeper",
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
	SilenceUsage: true,
	Short:        "Secure password manager CLI",
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
	dataDirPath = filepath.Join(dataDirPath, ".gophkeeper")
	rootCmd.PersistentFlags().StringVarP(&serverAddr, "server", "s", "localhost:50051", "gRPC server address")
	rootCmd.PersistentFlags().StringVarP(&dataDirPath, "data-dir", "d", dataDirPath, "directory for storing client data")
	rootCmd.PersistentFlags().BoolVar(&insecure, "insecure", false, "disable TLS")
	rootCmd.PersistentFlags().StringVar(&tlsCA, "tls-ca", "cert.pem", "path to self-signed CA certificate")
	// auth
	rootCmd.AddCommand(registerCmd)
	rootCmd.AddCommand(loginCmd)
	// vault
	rootCmd.AddCommand(addCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(getCmd)
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(deleteCmd)
	// sync
	rootCmd.AddCommand(syncCmd)
	// version
	rootCmd.AddCommand(versionCmd)
}
