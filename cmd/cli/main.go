package main

import (
	"fmt"
	"os"

	"github.com/theolujay/appa/internal/cli"
)

func main() {
	defer func() {
		if err := recover(); err != nil {
			fmt.Fprintf(os.Stderr, "unexpected error: %v\n", err)
			os.Exit(1)
		}
	}()

	app := cli.NewApp()
	if err := app.Execute(); err != nil {
		os.Exit(1)
	}
}
