package main

import (
	"fmt"
	"os"

	"github.com/brankas/assetgen/gen"
)

func main() {
	if err := gen.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
