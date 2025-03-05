package main

import (
	"log"
	"os"
	"sync"
	"time"

	gobber "github.com/manojpawarsj12/gobber/internal"
	"github.com/urfave/cli/v2"
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
				Action:  InstallCommand,
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
	if len(packageNames) == 0 {
		packageData, err := gobber.ReadPackageJSON("package.json")
		if err != nil {
			log.Printf("Error reading package.json: %v", err)
			return cli.Exit("Failed to read package.json", 1)
		}
		packageNames = append(packageNames, gobber.GetMapKeys(packageData.Dependencies)...)
		packageNames = append(packageNames, gobber.GetMapKeys(packageData.DevDependencies)...)
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
	var mut sync.Mutex
	installedVersions := make(map[string]string)
	wd, _ := os.Getwd()
	done := make(chan error, len(packageNames))

	for _, packageName := range packageNames {
		packageDetail, err := gobber.Parse(packageName)
		if err != nil {
			return err
		}
		go func(packageDetail *gobber.PackageDetails) {
			err := gobber.Execute(packageDetail, &mut, &installedVersions, wd)
			done <- err
		}(packageDetail)
	}

	for range packageNames {
		if err := <-done; err != nil {
			return err
		}
	}
	close(done)

	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)
	log.Printf("Installed total packages: %d", len(installedVersions))
	return nil
}
