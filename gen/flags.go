package gen

import (
	"flag"
	"runtime"
	"time"
)

// Flags holds config flags for generating static assets.
type Flags struct {
	Wd             string
	Verbose        bool
	Node           string
	NodeBin        string
	Yarn           string
	YarnBin        string
	Cache          string
	Build          string
	NodeModules    string
	NodeModulesBin string
	YarnUpgrade    bool
	YarnLatest     bool
	Assets         string
	Dist           string
	Script         string
	PackManifest   string
	PackMask       string
	Ttl            time.Duration
	Workers        int
	TFuncName      string
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
	fs.BoolVar(&f.YarnUpgrade, "upgrade", false, "toggle upgrade")
	fs.BoolVar(&f.YarnLatest, "latest", false, "toggle upgrade latest")
	fs.StringVar(&f.Assets, "assets", "", "assets path")
	fs.StringVar(&f.Dist, "dist", "", "assets dist dir")
	fs.StringVar(&f.Script, "script", "", "assets script")
	fs.StringVar(&f.PackManifest, "pack-manifest", "manifest.json", "pack manifest name")
	fs.StringVar(&f.PackMask, "pack-mask", "{{path[:6]}}.{{hash[:6]}}.{{ext}}", "pack file mask")
	fs.DurationVar(&f.Ttl, "ttl", 24*7*time.Hour, "ttl for retrieved dependencies (node, yarn)")
	fs.IntVar(&f.Workers, "workers", runtime.NumCPU()+1, "number of workers")
	fs.StringVar(&f.TFuncName, "trans", "T", "trans func name")
	return fs
}
