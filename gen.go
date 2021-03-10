// +build ignore

package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	"github.com/kenshaw/assetgen/pack"
)

func main() {
	pkg := flag.String("pkg", "github.com/kenshaw/assetgen", "package")
	dir := flag.String("dir", "gen/gentpl", "directory with files")
	out := flag.String("out", "gen/gentpl.go", "out file name")
	flag.Parse()
	if err := run(*pkg, *dir, *out); err != nil {
		log.Fatalf("error: %v", err)
	}
}

func run(pkg, dir, out string) error {
	pkg = filepath.Join(os.Getenv("GOPATH"), "src", pkg)
	out = filepath.Join(pkg, out)
	p := pack.New()
	err := filepath.Walk(filepath.Join(pkg, dir), func(n string, fi os.FileInfo, err error) error {
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
