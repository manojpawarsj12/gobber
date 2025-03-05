package main

import (
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	gobber "github.com/manojpawarsj12/gobber/internal"
	"github.com/urfave/cli/v2"
)

const (
	maxConcurrentRequests = 10              // Limit concurrent requests
	requestDelay          = 1 * time.Second // Delay between requests
)

var (
	requestSemaphore = make(chan struct{}, maxConcurrentRequests)
	rateLimiter      = time.Tick(requestDelay)
)

func main() {
	app := &cli.App{
		Name:  "gobber",
		Usage: "A package manager for Go",
		Commands: []*cli.Command{
			{
				Name:    "install",
				Aliases: []string{"i"},
				Usage:   "Install packages",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "dev",
						Aliases: []string{"D"},
						Usage:   "Install dev dependencies only",
					},
				},
				Action: InstallCommand,
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func InstallCommand(c *cli.Context) error {
	packageNames := c.Args().Slice()
	devOnly := c.Bool("dev")
	
	if len(packageNames) == 0 {
		packageData, err := gobber.ReadPackageJSON("package.json")
		if err != nil {
			log.Printf("Error reading package.json: %v", err)
			return cli.Exit("Failed to read package.json", 1)
		}

		if devOnly {
			// Only install dev dependencies
			packageNames = append(packageNames, gobber.GetMapKeys(packageData.DevDependencies)...)
		} else {
			// Install regular dependencies by default
			packageNames = append(packageNames, gobber.GetMapKeys(packageData.Dependencies)...)
		}
	}

	err := installPackage(packageNames)
	if err != nil {
		log.Printf("Error installing packages: %v", err)
		return cli.Exit("Installation failed", 1)
	}
	return nil
}

func installPackage(packageNames []string) error {
	log.Printf("Installing packages: %s\n", packageNames)
	start := time.Now()
	var wg sync.WaitGroup
	errChan := make(chan error, len(packageNames))
	installedVersions := make(map[string]string)
	var mut sync.Mutex
	wd, _ := os.Getwd()

	for _, packageName := range packageNames {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			<-rateLimiter
			requestSemaphore <- struct{}{}
			defer func() { <-requestSemaphore }()

			packageDetail, err := gobber.Parse(name)
			if err != nil {
				errChan <- err
				return
			}

			err = gobber.Execute(packageDetail, &mut, &installedVersions, wd)
			if err != nil {
				errChan <- fmt.Errorf("package %s: %w", name, err)
			}
		}(packageName)
	}

	go func() {
		wg.Wait()
		close(errChan)
	}()

	for err := range errChan {
		if err != nil {
			log.Printf("Installation error: %v", err)
			return cli.Exit("Installation failed", 1)
		}
	}

	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)
	log.Printf("Installed total packages: %d", len(installedVersions))
	return nil
}
