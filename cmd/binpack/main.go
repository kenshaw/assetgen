package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kenshaw/binpack"
)

func main() {
	if err := binpack.Translate(parseArgs()); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// parseArgs create s a new, filled configuration instance by reading and
// parsing command line options.
//
// This function exits the program with an error, if any of the command line
// options are incorrect.
func parseArgs() *binpack.Binpack {
	var version bool

	bp := binpack.New()

	flag.Usage = func() {
		fmt.Printf("Usage: %s [options] <input directories>\n\n", os.Args[0])
		flag.PrintDefaults()
	}

	flag.BoolVar(&bp.Debug, "debug", bp.Debug, "Do not embed the assets, but provide the embedding API. Contents will still be loaded from disk.")
	flag.BoolVar(&bp.Dev, "dev", bp.Dev, "Similar to debug, but does not emit absolute paths. Expects a rootDir variable to already exist in the generated code's package.")
	flag.StringVar(&bp.Tags, "tags", bp.Tags, "Optional set of build tags to include.")
	flag.StringVar(&bp.Prefix, "prefix", bp.Prefix, "Optional path prefix to strip off asset names.")
	flag.StringVar(&bp.Package, "pkg", bp.Package, "Package name to use in the generated code.")
	flag.BoolVar(&bp.NoMemCopy, "nomemcopy", bp.NoMemCopy, "Use a .rodata hack to get rid of unnecessary memcopies. Refer to the documentation to see what implications this carries.")
	flag.BoolVar(&bp.NoCompress, "nocompress", bp.NoCompress, "Assets will *not* be GZIP compressed when this flag is specified.")
	flag.BoolVar(&bp.NoMetadata, "nometadata", bp.NoMetadata, "Assets will not preserve size, mode, and modtime info.")
	flag.UintVar(&bp.Mode, "mode", bp.Mode, "Optional file mode override for all files.")
	flag.Int64Var(&bp.ModTime, "modtime", bp.ModTime, "Optional modification unix timestamp override for all files.")
	flag.StringVar(&bp.Output, "o", bp.Output, "Optional name of the output file to be generated.")
	flag.BoolVar(&version, "version", false, "Displays version information.")

	ignore := make([]string, 0)
	flag.Var((*AppendSliceValue)(&ignore), "ignore", "Regex pattern to ignore")

	flag.Parse()

	for _, pattern := range ignore {
		bp.Ignore = append(bp.Ignore, regexp.MustCompile(pattern))
	}

	if version {
		fmt.Printf("%s\n", Version())
		os.Exit(0)
	}

	// ensure we have input paths
	if flag.NArg() == 0 {
		fmt.Fprintf(os.Stderr, "Missing <input dir>\n\n")
		flag.Usage()
		os.Exit(1)
	}

	// create input configurations
	bp.Input = make([]binpack.Input, flag.NArg())
	for i := 0; i < flag.NArg(); i++ {
		bp.Input = append(bp.Input, parseInput(flag.Arg(i)))
	}

	return bp
}

// parseInput determines whether the given path has a recrusive indicator and
// returns a new path with the recursive indicator chopped off if it does.
//
//  ex:
//      /path/to/foo/...    -> (/path/to/foo, true)
//      /path/to/bar        -> (/path/to/bar, false)
func parseInput(path string) binpack.Input {
	var recursive bool
	if strings.HasSuffix(path, "/...") {
		recursive, path = true, strings.TrimSuffix(path, "/...")
	}

	return binpack.Input{
		Path:      filepath.Clean(path),
		Recursive: recursive,
	}
}
