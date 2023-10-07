package gobber

import (
	"log"
	"os"
)

func ParseCommands() {
	switch os.Args[1] {
	case "install":
		if len(os.Args) < 3 {
			log.Println("Error: Package name is required for the install command")
			os.Exit(1)
		}
		packageName := os.Args[2]
		installPackage(packageName)
	default:
		log.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func installPackage(packageName string) {
	log.Printf("Installing package: %s\n", packageName)
	packageDetails, _ := Parse(packageName)
	Execute(packageDetails)
	// npmVersionData, _ := NpmRegistryVersionData(&packageName)}
}
