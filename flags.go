package assetgen

import (
	"flag"
	"runtime"
)

// Flags holds config flags for generating static assets.
type Flags struct {
	Wd string

	Verbose bool
	Yarn    string

	Cache string
	Build string
	Dist  string

	Node    string
	NodeBin string

	Assets string
	Script string

	//Ttl      time.Duration
	//Env string
	//NoUpdate bool

	Workers int

	TFuncName string
}

// NewFlags creates a set of flags for use by assetgen.
func NewFlags(wd string) *Flags {
	return &Flags{
		Wd: wd,
	}
}

// FlagSet returns a standard flag set for assetgen flags.
func (f *Flags) FlagSet(name string, errorHandling flag.ErrorHandling) *flag.FlagSet {
	fs := flag.NewFlagSet(name, errorHandling)

	fs.BoolVar(&f.Verbose, "v", true, "toggle verbose")
	fs.StringVar(&f.Yarn, "yarn", "", "path to yarn")

	fs.StringVar(&f.Cache, "cache", "", "cache directory")
	fs.StringVar(&f.Build, "build", "", "build directory")
	fs.StringVar(&f.Dist, "dist", "", "distribution directory")

	fs.StringVar(&f.Node, "node", "", "node path")
	fs.StringVar(&f.NodeBin, "node-bin", "", "node bin path")

	fs.StringVar(&f.Assets, "assets", "", "assets directory")
	fs.StringVar(&f.Script, "script", "", "script file")

	//fs.DurationVar(&f.Ttl, "ttl", 24*7*time.Hour, "ttl on updating static asset files")
	//fs.StringVar(&f.Env, "env", os.Getenv("ENV"), "environment")
	//fs.BoolVar(&f.NoUpdate, "noupdate", false, "no update")

	fs.IntVar(&f.Workers, "workers", runtime.NumCPU()+1, "number of workers")

	fs.StringVar(&f.TFuncName, "trans", "T", "trans func name")

	return fs
}
