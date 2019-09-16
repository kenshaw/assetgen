package gen

import (
	"flag"
	"runtime"
	"time"
)

// Flags holds config flags for generating static assets.
type Flags struct {
	Wd string

	Verbose bool

	Node    string
	NodeBin string

	Yarn    string
	YarnBin string

	Cache string
	Build string

	NodeModules    string
	NodeModulesBin string

	YarnUpgrade bool
	YarnLatest  bool

	Assets string
	Script string

	ManifestName string

	Ttl time.Duration

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
	fs.StringVar(&f.Node, "node", "", "path to node executable")
	fs.StringVar(&f.Yarn, "yarn", "", "path to yarn executable")

	fs.StringVar(&f.Cache, "cache", "", "cache directory")
	fs.StringVar(&f.Build, "build", "", "build directory")

	fs.StringVar(&f.NodeModules, "node-modules", "", "node_modules path")
	fs.StringVar(&f.NodeModulesBin, "node-modules-bin", "", "node_modules/.bin path")

	fs.StringVar(&f.Assets, "assets", "", "assets path")
	fs.StringVar(&f.Script, "script", "", "script file")

	fs.BoolVar(&f.YarnUpgrade, "upgrade", false, "toggle upgrade")
	fs.BoolVar(&f.YarnLatest, "latest", false, "toggle latest on upgrade")

	fs.StringVar(&f.ManifestName, "manifest-name", "%s[:4]%s[:4]%s", "manifest name")

	fs.DurationVar(&f.Ttl, "ttl", 24*7*time.Hour, "ttl for retrieved dependencies (node, yarn, geoip)")

	fs.IntVar(&f.Workers, "workers", runtime.NumCPU()+1, "number of workers")

	fs.StringVar(&f.TFuncName, "trans", "T", "trans func name")

	return fs
}
