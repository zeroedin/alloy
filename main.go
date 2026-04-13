package main

import (
	"os"

	"github.com/zeroedin/alloy/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
