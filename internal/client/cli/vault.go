// Package cli предоставляет CLI команды для управления секретами.
// Реализует операции добавления, просмотра, обновления и удаления элементов.
package cli

import (
	"errors"
	"fmt"
	"syscall"

	gophkeeperv1 "github.com/prbllm/goph-keeper/api/proto/gophkeeper/v1"
	"github.com/prbllm/goph-keeper/internal/client/app"
	"github.com/prbllm/goph-keeper/internal/client/model"
	"github.com/prbllm/goph-keeper/internal/client/storage"
	"github.com/prbllm/goph-keeper/internal/client/transport"
	"golang.org/x/term"

	"github.com/spf13/cobra"
)

// addCmd — команда добавления нового секрета в хранилище.
// Поддерживает типы: text, credential, card.
// Запрашивает пароль для расшифровки ключа и добавляет элемент локально.
var addCmd = &cobra.Command{
	Use:   "add [type] [data]",
	Short: "Add secret",
	Args:  cobra.RangeArgs(2, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		var (
			itemType gophkeeperv1.ItemType
			payload  []byte
			err      error
		)

		switch args[0] {
		case "text":
			if len(args) != 2 {
				err = errors.New("accepts [data]")
			} else {
				itemType = gophkeeperv1.ItemType_ITEM_TYPE_TEXT
				payload = []byte(args[1])
			}
		case "credential":
			if len(args) != 3 {
				err = errors.New("accepts [login] [password]")
			} else {
				itemType = gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL
				p := model.CredentialPayload{
					Login:    args[1],
					Password: args[2],
				}
				payload, err = model.EncodePayload(p)
			}
		case "card":
			if len(args) != 5 {
				err = errors.New("accepts [number] [holder] [expire] [cvv]")
			} else {
				itemType = gophkeeperv1.ItemType_ITEM_TYPE_CARD
				p := model.CardPayload{
					Number: args[1],
					Holder: args[2],
					Exp:    args[3],
					CVV:    args[4],
				}
				payload, err = model.EncodePayload(p)
			}
		default:
			err = errors.New("accepts text/credential/card")
		}
		if err != nil {
			return err
		}

		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.New(dataDirPath)
		if err != nil {
			return err
		}
		state, err := localStorage.Load()
		if err != nil {
			return err
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

		password, err := promptPassword()
		if err != nil {
			return err
		}

		if err = app.Unlock(password); err != nil {
			return err
		}

		if len(app.DEK) == 0 {
			return errors.New("login required (DEK missing)")
		}

		if err = app.AddItem(itemType, []byte("title"), nil, payload); err != nil {
			return err
		}

		fmt.Println("Add successfully. Pending sync...")

		return nil
	},
}

// listCmd — команда просмотра списка всех секретов.
// Запрашивает пароль, расшифровывает ключ и выводит список элементов.
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List secrets",
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.New(dataDirPath)
		if err != nil {
			return err
		}
		state, err := localStorage.Load()
		if err != nil {
			return err
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

		password, err := promptPassword()
		if err != nil {
			return err
		}

		if err = app.Unlock(password); err != nil {
			return err
		}

		items, err := app.ListItems()
		if err != nil {
			return err
		}

		if len(items) == 0 {
			fmt.Println("No items")
			return nil
		}

		for _, it := range items {
			title, meta, _, err := app.DecryptItem(it)
			if err != nil {
				return err
			}

			fmt.Println("ID:", it.ID)
			fmt.Println("Title:", title)
			fmt.Println("Meta:", meta)
			fmt.Println("---")
		}

		return nil
	},
}

// getCmd — команда получения деталей конкретного секрета.
// Принимает идентификатор элемента, запрашивает пароль и выводит все поля.
var getCmd = &cobra.Command{
	Use:   "get [id]",
	Short: "Get secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.New(dataDirPath)
		if err != nil {
			return err
		}
		state, err := localStorage.Load()
		if err != nil {
			return err
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

		password, err := promptPassword()
		if err != nil {
			return err
		}

		if err = app.Unlock(password); err != nil {
			return err
		}

		item, ok := app.GetItem(id)
		if !ok {
			return errors.New("item not found")
		}

		title, meta, payload, err := app.DecryptItem(item)
		if err != nil {
			return err
		}

		fmt.Println("ID:", item.ID)
		fmt.Println("Title:", title)
		fmt.Println("Meta:", meta)
		fmt.Println("Data:", payload)

		return nil
	},
}

// updateCmd — команда обновления существующего секрета.
// Принимает идентификатор и новые данные, запрашивает пароль и обновляет элемент.
var updateCmd = &cobra.Command{
	Use:   "update [id] [type] [data]",
	Short: "Update secret",
	Args:  cobra.RangeArgs(3, 5),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]

		var (
			itemType gophkeeperv1.ItemType
			payload  []byte
			err      error
		)

		switch args[1] {

		case "text":
			if len(args) != 3 {
				err = errors.New("accepts [data]")
			} else {
				itemType = gophkeeperv1.ItemType_ITEM_TYPE_TEXT
				payload = []byte(args[2])
			}
		case "credential":
			if len(args) != 4 {
				err = errors.New("accepts [login] [password]")
			} else {
				itemType = gophkeeperv1.ItemType_ITEM_TYPE_CREDENTIAL
				p := model.CredentialPayload{
					Login:    args[2],
					Password: args[3],
				}
				payload, err = model.EncodePayload(p)
			}
		case "card":
			if len(args) != 6 {
				err = errors.New("accepts [number] [holder] [expire] [cvv]")
			} else {
				itemType = gophkeeperv1.ItemType_ITEM_TYPE_CARD
				p := model.CardPayload{
					Number: args[2],
					Holder: args[3],
					Exp:    args[4],
					CVV:    args[5],
				}
				payload, err = model.EncodePayload(p)
			}
		default:
			err = errors.New("accepts text/credential/card")
		}
		if err != nil {
			return err
		}

		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.New(dataDirPath)
		if err != nil {
			return err
		}
		state, err := localStorage.Load()
		if err != nil {
			return err
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

		password, err := promptPassword()
		if err != nil {
			return err
		}

		if err = app.Unlock(password); err != nil {
			return err
		}

		if err = app.UpdateItem(id, itemType, []byte("title"), nil, payload); err != nil {
			return err
		}

		fmt.Println("Updated successfully. Pending sync...")

		return nil
	},
}

// deleteCmd — команда удаления секрета из хранилища.
// Принимает идентификатор элемента, запрашивает пароль и удаляет элемент.
var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete secret",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dataDirPath, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			return err
		}

		localStorage, err := storage.New(dataDirPath)
		if err != nil {
			return err
		}
		state, err := localStorage.Load()
		if err != nil {
			return err
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

		password, err := promptPassword()
		if err != nil {
			return err
		}

		if err = app.Unlock(password); err != nil {
			return err
		}

		if err = app.DeleteItem(args[0]); err != nil {
			return err
		}

		fmt.Println("Deleted successfully. Pending sync...")

		return nil
	},
}

// promptPassword запрашивает пароль у пользователя без отображения ввода.
// Использует терминальный режим для скрытия ввода пароля.
// Возвращает введённый пароль в виде строки или ошибку.
func promptPassword() (string, error) {
	fmt.Print("Enter password: ")

	bytePassword, err := term.ReadPassword(int(syscall.Stdin))
	fmt.Println() // перенос строки после ввода

	if err != nil {
		return "", err
	}

	return string(bytePassword), nil
}
