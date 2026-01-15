package main

import (
	"os"

	"github.com/sultano/coffer/cmd"
	"github.com/fatih/color"
)

func main() {
	if err := cmd.Execute(); err != nil {
		color.Red("Error: %s", err.Error())
		os.Exit(1)
	}
}
