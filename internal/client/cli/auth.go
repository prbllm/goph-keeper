// Package cli предоставляет командный интерфейс для работы с клиентом.
// Реализует команды аутентификации, управления секретами и синхронизации.
package cli

import (
	"fmt"

	"github.com/prbllm/goph-keeper/internal/client/app"
	"github.com/prbllm/goph-keeper/internal/client/storage"
	"github.com/prbllm/goph-keeper/internal/client/transport"

	"github.com/spf13/cobra"
)

// registerCmd — команда регистрации нового пользователя.
// Принимает два аргумента: логин и пароль.
// Создаёт подключение к серверу, инициализирует приложение и регистрирует пользователя.
var registerCmd = &cobra.Command{
	Use:     "register [login] [password]",
	Short:   "Register user",
	GroupID: "auth",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.NewSQLiteStorage(dataDirPath)
		if err != nil {
			return err
		}
		defer localStorage.Close()

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

		client, err := transport.New(serverAddr, "", transport.DialOptions{Insecure: insecure, CAPath: tlsCA})
		if err != nil {
			return err
		}
		defer client.Close()

		app, err := app.New(client, localStorage)
		if err != nil {
			return err
		}

		if err := app.Register(cmd.Context(), args[0], args[1]); err != nil {
			return err
		}

		fmt.Println("Register successful")

		return nil
	},
}

// loginCmd — команда входа существующего пользователя.
// Принимает два аргумента: логин и пароль.
// Создаёт подключение к серверу, инициализирует приложение и выполняет вход.
var loginCmd = &cobra.Command{
	Use:     "login [login] [password]",
	Short:   "Login user",
	GroupID: "auth",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.NewSQLiteStorage(dataDirPath)
		if err != nil {
			return err
		}
		defer localStorage.Close()

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

		client, err := transport.New(serverAddr, "", transport.DialOptions{Insecure: insecure, CAPath: tlsCA})
		if err != nil {
			return err
		}
		defer client.Close()

		app, err := app.New(client, localStorage)
		if err != nil {
			return err
		}

		if err := app.Login(cmd.Context(), args[0], args[1]); err != nil {
			return err
		}

		fmt.Println("Login successful")

		return nil
	},
}
