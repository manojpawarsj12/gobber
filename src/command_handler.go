package gobber

import (
	"log"
	"os"
	"sync"
	"time"
)

func ParseCommands() {
	switch os.Args[1] {
	case "install":
		if len(os.Args) < 3 {
			log.Println("Error: Package name is required for the install command")
			os.Exit(1)
		}
		packageNames := os.Args[2:]
		installPackage(packageNames)
	default:
		log.Printf("Unknown command: %s\n", os.Args[1])
		os.Exit(1)
	}
}

func installPackage(packageNames []string) {
	log.Printf("Installing packages: %s\n", packageNames)
	start := time.Now()
	var mut sync.Mutex
	var InstalledVersionsMutex = make(map[string]string)
	wd, _ := os.Getwd()
	done := make(chan bool, len(packageNames))
	defer close(done)
	for _, packageName := range packageNames {
		packageDetail, _ := Parse(packageName)
		go func() {
			Execute(packageDetail, &mut, &InstalledVersionsMutex, wd)
			done <- true
		}()
	}
	for range packageNames {
		<-done
	}
	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)
	log.Println("Installed total packages: ", InstalledVersionsMutex, len(InstalledVersionsMutex))
}
