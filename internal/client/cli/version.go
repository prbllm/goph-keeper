// Package cli предоставляет CLI команду для отображения версии приложения.
package cli

import (
	"fmt"

	"github.com/prbllm/goph-keeper/pkg/version"

	"github.com/spf13/cobra"
)

// versionCmd — команда вывода информации о версии клиента.
// Отображает номер версии и дату сборки.
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Version: %s\nBuild date: %s\n", version.Version, version.BuildDate)
	},
}
