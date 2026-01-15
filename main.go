package main

import (
	"os"

	"github.com/fatih/color"
	"github.com/sultano/coffer/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		color.Red("Error: %s", err.Error())
		os.Exit(1)
	}
}
