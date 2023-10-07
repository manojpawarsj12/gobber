package main

import (
	"log"
	"os"

	gobber "github.com/manojpawarsj12/gobber/src"
)

func main() {
	if len(os.Args) < 2 {
		log.Println("Usage: gobber [command]")
		log.Println("Available commands: install")
		os.Exit(1)
	}
	gobber.ParseCommands()

}
