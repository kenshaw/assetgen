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
	flagPkg = flag.String("pkg", "github.com/brankas/assetgen", "package")
	flagDir = flag.String("dir", "gen/gentpl", "directory with files")
	flagOut = flag.String("out", "gen/gentpl.go", "out file name")
)

func main() {
	flag.Parse()

	if err := run(); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run() error {
	pkg := filepath.Join(os.Getenv("GOPATH"), "src", *flagPkg)
	out := filepath.Join(pkg, *flagOut)

	p := pack.New()
	err := filepath.Walk(filepath.Join(pkg, *flagDir), func(n string, fi os.FileInfo, err error) error {
		fn := filepath.Base(n)
		switch {
		case err != nil:
			return err
		case fi.IsDir() || fn == out:
			return nil
		}
		return p.AddFile(fn, n)
	})
	if err != nil {
		return err
	}
	return p.WriteTo(out, "files")
}
