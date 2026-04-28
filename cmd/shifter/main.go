package main

import (
	"os"

	"github.com/shifter-io/shifter/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
