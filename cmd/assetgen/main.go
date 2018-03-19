package main

import (
	"fmt"
	"os"

	"github.com/brankas/assetgen"
)

func main() {
	err := assetgen.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
