package main

import (
	"fmt"
	command_handler "gobber/src"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gobber [command]")
		fmt.Println("Available commands: install")
		os.Exit(1)
	}
	command_handler.ParseCommands()
}
