// Package main is the entry point for the LLM gateway server.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/0xfig-labs/thinroute/internal/command"
	"github.com/0xfig-labs/thinroute/run"
)

func main() {
	if isManagementCommand(os.Args[1:]) {
		if err := command.Run(os.Args); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "serve" {
		args = args[1:]
	}
	err := run.Run(context.Background(), run.Options{
		ProductName: "github.com/0xfig-labs/thinroute",
		Args:        args,
	})
	if code := run.ExitCode(err); code != 0 {
		os.Exit(code)
	}
}

func isManagementCommand(args []string) bool {
	commands := map[string]struct{}{
		"config":         {},
		"usage":          {},
		"providers":      {},
		"models":         {},
		"virtual-models": {},
		"auth":           {},
		"doctor":         {},
	}
	for _, arg := range args {
		if _, ok := commands[arg]; ok {
			return true
		}
	}
	return false
}
