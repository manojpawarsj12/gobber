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
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:     "package",
						Aliases:  []string{"p"},
						Usage:    "Package name(s) to install",
						Required: true,
					},
				},
			},
		},
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}
}

func InstallCommand(c *cli.Context) error {
	packageNames := c.StringSlice("package")
	if len(packageNames) == 0 {
		log.Println("Error: Package name is required for the install command")
		return cli.Exit("Package name is required", 1)
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
	defer close(done)

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

	elapsed := time.Since(start)
	log.Printf("Took %s", elapsed)
	log.Printf("Installed total packages: %d", len(installedVersions))
	return nil
}
