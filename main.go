package main

import (
	"fmt"
	"os"

	gobber "github.com/manojpawarsj12/gobber/src"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: gobber [command]")
		fmt.Println("Available commands: install")
		os.Exit(1)
	}
	gobber.ParseCommands()

}
