// +build ignore

package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/brankas/assetgen/pack"
)

var (
	flagPkg  = flag.String("pkg", "github.com/brankas/assetgen/gen", "package")
	flagFile = flag.String("file", "gentpl.go", "generated file name")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	p := pack.New(filepath.Base(*flagPkg))

	dir := filepath.Join(os.Getenv("GOPATH"), "src", *flagPkg)
	err := filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
		fn := filepath.Base(n)
		switch {
		case err != nil:
			return err
		case fi.IsDir() || fn == *flagFile:
			return nil
		}
		return p.AddFile(fn, n)
	})
	if err != nil {
		return err
	}

	return p.WriteTo(filepath.Join(dir, *flagFile), "files")
}
