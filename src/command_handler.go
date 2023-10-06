package gobber

import (
	"fmt"
	"os"
)

func ParseCommands() {
	switch os.Args[1] {
	case "install":
		if len(os.Args) < 3 {
			fmt.Println("Error: Package name is required for the install command")
			os.Exit(1)
		}
		packageName := os.Args[2]
		installPackage(packageName)
	default:
		fmt.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func installPackage(packageName string) {
	fmt.Printf("Installing package: %s\n", packageName)
	npmData, _ := NpmRegistry(&packageName)
	packageDetails, _ := Parse(packageName)
	Execute(packageDetails)
	// npmVersionData, _ := NpmRegistryVersionData(&packageName)

	fmt.Println(npmData)
}
