package main

import (
	"fmt"
	"os"
)

func version() string { return "ghrun dev" }

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
