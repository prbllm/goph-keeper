package main

import (
	"fmt"
	"os"

	"github.com/prbllm/goph-keeper/internal/server/app"
)

func main() {
	if err := app.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
