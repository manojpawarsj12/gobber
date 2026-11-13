package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	gobber "github.com/manojpawarsj12/gobber/internal"
	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "gobber",
		Usage: "a fast, npm-compatible package installer",
		Commands: []*cli.Command{
			{
				Name:      "install",
				Aliases:   []string{"i", "add"},
				Usage:     "install package.json dependencies, or add packages",
				ArgsUsage: "[package[@spec] ...]",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "save-dev", Aliases: []string{"D", "dev"}, Usage: "save added packages to devDependencies"},
					&cli.BoolFlag{Name: "production", Aliases: []string{"prod"}, Usage: "skip devDependencies (lockfile is not rewritten)"},
				},
				Action: InstallCommand,
			},
			{
				Name:  "store",
				Usage: "print the global store path",
				Action: func(*cli.Context) error {
					store, err := storeDir()
					fmt.Println(store)
					return err
				},
			},
		},
	}
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, "gobber:", err)
		os.Exit(1)
	}
}

func storeDir() (string, error) {
	if s := os.Getenv("GOBBER_STORE"); s != "" {
		return filepath.Abs(s)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(cache, "gobber", "store"), nil
}

func InstallCommand(c *cli.Context) error {
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	store, err := storeDir()
	if err != nil {
		return err
	}
	registry := os.Getenv("GOBBER_REGISTRY")
	if registry == "" {
		registry = gobber.DefaultRegistry
	}
	// urfave/cli stops at the first positional arg; accept npm-style trailing flags too.
	saveDev, production := c.Bool("save-dev"), c.Bool("production")
	var args []string
	for _, a := range c.Args().Slice() {
		switch a {
		case "-D", "--save-dev", "--dev":
			saveDev = true
		case "--production", "--prod":
			production = true
		default:
			if len(a) > 1 && a[0] == '-' {
				return fmt.Errorf("unknown flag %s", a)
			}
			args = append(args, a)
		}
	}
	start := time.Now()
	res, err := gobber.Install(gobber.Options{
		Dir:         dir,
		Store:       store,
		Registry:    registry,
		Args:        args,
		SaveDev:     saveDev,
		Production:  production,
		Concurrency: 50,
	})
	if err != nil {
		return err
	}
	fmt.Printf("%d packages (%d downloaded) in %s\n", res.Packages, res.Downloaded, time.Since(start).Round(time.Millisecond))
	return nil
}
