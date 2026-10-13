package main

import (
	"errors"
	"os"

	"github.com/fatih/color"
	"github.com/sultano/coffer/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		var exitErr *cmd.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.Code)
		}
		color.Red("Error: %s", err.Error())
		os.Exit(1)
	}
}
