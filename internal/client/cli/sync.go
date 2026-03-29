// Package cli предоставляет CLI команду для синхронизации с сервером.
package cli

import (
	"errors"
	"fmt"

	"github.com/prbllm/goph-keeper/internal/client/app"
	"github.com/prbllm/goph-keeper/internal/client/storage"
	"github.com/prbllm/goph-keeper/internal/client/transport"

	"github.com/spf13/cobra"
)

// syncCmd — команда синхронизации локальных изменений с сервером.
// Загружает состояние аутентификации, создаёт приложение и выполняет синхронизацию.
// Требует предварительной аутентификации пользователя.
var syncCmd = &cobra.Command{
	Use:     "sync",
	Short:   "Sync with server",
	GroupID: "sync",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.NewSQLiteStorage(dataDirPath)
		if err != nil {
			return err
		}
		state, err := localStorage.Load()
		if err != nil {
			return err
		}

		if state.AccessToken == "" {
			return errors.New("not logged in")
		}

		serverAddr, err := cmd.Flags().GetString("server")
		if err != nil {
			return err
		}

		insecure, err := cmd.Flags().GetBool("insecure")
		if err != nil {
			return err
		}

		tlsCA, err := cmd.Flags().GetString("tls-ca")
		if err != nil {
			return err
		}

		client, err := transport.New(serverAddr, state.AccessToken, transport.DialOptions{Insecure: insecure, CAPath: tlsCA})
		if err != nil {
			return err
		}
		defer client.Close()

		app, err := app.New(client, localStorage)
		if err != nil {
			return err
		}

		if err := app.Sync(cmd.Context()); err != nil {
			return err
		}

		fmt.Println("Synced successfully")

		return nil
	},
}
