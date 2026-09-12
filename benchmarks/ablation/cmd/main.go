package main

import (
	"fmt"
	"os"

	"toron/benchmarks/ablation"
)

func main() {
	if err := ablation.RunCLI(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
