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
	var wg sync.WaitGroup
	start := time.Now()
	var mut sync.Mutex
	var InstalledVersionsMutex = make(map[string]string)
	wd, _ := os.Getwd()
	for _, packageName := range packageNames {
		wg.Add(1)
		packageDetail, _ := Parse(packageName)
		go Execute(packageDetail, &wg, &mut, &InstalledVersionsMutex, wd)
	}
	wg.Wait()
	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)
	log.Println("Installed total packages: ", len(InstalledVersionsMutex))
	// npmVersionData, _ := NpmRegistryVersionData(&packageName)}
}
