package main

import (
	"os"

	"github.com/theolujay/appa/internal/cli"
)

func main() {
	app := cli.NewApp()
	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}
