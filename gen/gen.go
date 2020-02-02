package gen

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yookoala/realpath"
	"golang.org/x/xerrors"
)

const (
	nodeConstraint = ">=10.16.x"
	yarnConstraint = ">=1.17.x"

	cacheDir          = ".cache"
	buildDir          = "build"
	nodeModulesDir    = "node_modules"
	nodeModulesBinDir = ".bin"
	assetsDir         = "assets"

	productionEnv  = "production"
	developmentEnv = "development"

	scriptName   = "assets.anko"
	assetsFile   = "assets.go"
	fontsDir     = "fonts"
	geoipDir     = "geoip"
	localesDir   = "locales"
	imagesDir    = "images"
	jsDir        = "js"
	sassDir      = "sass"
	cssDir       = "css"
	sassJs       = "sass.js"
	assetgenScss = "_assetgen.scss"
	templatesDir = "templates"

	nodeDistURL = "https://nodejs.org/dist"

	geoipURL = "https://download.maxmind.com/app/geoip_download?edition_id=GeoLite2-Country&license_key=%s"
)

// Run generates assets using the current working directory and default flags.
func Run() error {
	var err error

	// load working directory
	wd, err := os.Getwd()
	if err != nil {
		return xerrors.Errorf("could not determine working directory: %w", err)
	}

	// build flags
	flags := NewFlags(wd)
	fs := flags.FlagSet(filepath.Base(os.Args[0]), flag.ExitOnError)
	err = fs.Parse(os.Args[1:])
	if err != nil {
		return xerrors.Errorf("could not parse args: %w", err)
	}

	return Assetgen(flags)
}

// Assetgen generates assets based on the passed flags.
func Assetgen(flags *Flags) error {
	var err error

	// check working directory is usable
	wdfi, err := os.Stat(flags.Wd)
	if err != nil || !wdfi.IsDir() {
		return xerrors.Errorf("cannot read from working directory %q", flags.Wd)
	}
	wd, err := realpath.Realpath(flags.Wd)
	if err != nil {
		return xerrors.Errorf("could not determine real path for %s: %w", flags.Wd, err)
	}
	flags.Wd = wd

	// ensure workers is at least 1
	if flags.Workers < 1 {
		return xerrors.New("workers must be at least 1")
	}

	// ensure valid trans func name
	if !isValidIdentifier(flags.TFuncName) {
		return xerrors.New("invalid trans func name")
	}

	// ensure paths are set
	if flags.Cache == "" {
		flags.Cache = filepath.Join(flags.Wd, cacheDir)
	}
	if flags.Build == "" {
		flags.Build = filepath.Join(flags.Wd, buildDir)
	}
	if flags.NodeModules == "" {
		flags.NodeModules = filepath.Join(flags.Cache, nodeModulesDir)
	}
	if flags.NodeModulesBin == "" {
		flags.NodeModulesBin = filepath.Join(flags.NodeModules, nodeModulesBinDir)
	}
	if flags.Assets == "" {
		flags.Assets = filepath.Join(flags.Wd, assetsDir)
	}
	if flags.Script == "" {
		flags.Script = filepath.Join(flags.Assets, scriptName)
	}

	// set working directory
	if err = os.Chdir(flags.Wd); err != nil {
		return xerrors.Errorf("could not change to dir: %w", err)
	}

	// check setup
	if err = checkSetup(flags); err != nil {
		return err
	}

	// set PATH
	if err = os.Setenv("PATH", strings.Join([]string{
		filepath.Dir(flags.NodeBin),
		flags.NodeModulesBin,
		os.Getenv("PATH"),
	}, ":")); err != nil {
		return xerrors.Errorf("could not set PATH: %w", err)
	}

	// set NODE_PATH
	if err = os.Setenv("NODE_PATH", flags.NodeModules); err != nil {
		return xerrors.Errorf("could not set NODE_PATH: %w", err)
	}
	// load script
	s, err := LoadScript(flags)
	if err != nil {
		return xerrors.Errorf("unable to load script %s: %w", flags.Script, err)
	}

	// setup dependencies
	if err = s.ConfigDeps(); err != nil {
		return xerrors.Errorf("unable to configure dependencies: %w", err)
	}

	// fix links in node/.bin directory
	if err = fixNodeModulesBinLinks(flags); err != nil {
		return xerrors.Errorf("unable to fix bin links in %s: %w", flags.NodeModulesBin, err)
	}

	ctxt, cancel := context.WithCancel(context.Background())

	// start callback server
	sock, err := s.startCallbackServer(ctxt)
	if err != nil {
		return xerrors.Errorf("could not start callback server: %w", err)
	}
	defer func() {
		cancel()
		if err := os.RemoveAll(filepath.Dir(sock)); err != nil {
			warnf(flags, "could not remove %s: %w", sock, err)
		}
	}()

	// set ASSETGEN_SOCK
	if err = os.Setenv("ASSETGEN_SOCK", sock); err != nil {
		return xerrors.Errorf("could not set ASSETGEN_SOCK: %w", err)
	}

	// run script
	if err = s.Execute(); err != nil {
		return xerrors.Errorf("could not run script: %w", err)
	}

	return nil
}

// checkSetup checks that yarn is the correct version, and all necessary files
// and directories exist as expected.
func checkSetup(flags *Flags) error {
	var err error

	// ensure primary directories exist
	if err = checkDirs(flags, &flags.Cache, &flags.Build, &flags.Assets); err != nil {
		return xerrors.Errorf("unable to fix .cache build assets: %w", err)
	}

	// check node + yarn
	if err = checkNode(flags); err != nil {
		return err
	}
	if err = os.Setenv("PATH", filepath.Dir(flags.NodeBin)+":"+os.Getenv("PATH")); err != nil {
		return err
	}
	if err = checkYarn(flags); err != nil {
		return err
	}

	// determine if node_modules and yarn.lock is present
	var nodeModulesPresent, yarnLockPresent bool
	_, err = os.Stat(flags.NodeModules)
	switch {
	case err == nil:
		nodeModulesPresent = true
	}
	_, err = os.Stat(filepath.Join(flags.Wd, "yarn.lock"))
	switch {
	case err == nil:
		yarnLockPresent = true
	}

	// check dirs node_modules + node_modules/.bin
	if err = checkDirs(flags, &flags.NodeModules, &flags.NodeModulesBin); err != nil {
		return xerrors.Errorf("unable to fix node_modules and node_modules/.bin: %w", err)
	}

	// setup files
	if err = setupFiles(flags); err != nil {
		return xerrors.Errorf("unable to setup files: %w", err)
	}

	// do pure lockfile install
	if !nodeModulesPresent && yarnLockPresent {
		if err = run(flags, flags.YarnBin, "install", "--pure-lockfile", "--no-bin-links", "--modules-folder="+flags.NodeModules); err != nil {
			return xerrors.New("unable to install locked deps: please fix manually")
		}
	}

	// ensure node_modules and assets directories exist
	for _, d := range []struct{ n, v string }{
		{"assets", flags.Assets},
	} {
		_, err := filepath.Rel(flags.Wd, d.v)
		if err != nil || !isParentDir(flags.Wd, d.v) {
			return xerrors.Errorf("%s path must be subdirectory of working directory", d.n)
		}
	}

	// run yarn install
	if err = runSilent(flags, flags.YarnBin, "install", "--no-bin-links", "--modules-folder="+flags.NodeModules); err != nil {
		return xerrors.New("yarn is out of sync: please fix manually")
	}

	// run yarn upgrade
	if flags.YarnUpgrade {
		params := []string{"upgrade", "--no-bin-links", "--modules-folder=" + flags.NodeModules}
		if flags.YarnLatest {
			params = append(params, "--latest")
		}
		if err = runSilent(flags, flags.YarnBin, params...); err != nil {
			return xerrors.Errorf("unable to run yarn upgrade: %w", err)
		}
	}

	return nil
}

// checkDirs creates required directories and ensures node and assets are
// subdirectories of the working directory.
func checkDirs(flags *Flags, dirs ...*string) error {
	// make required directories
	for _, d := range dirs {
		v, err := filepath.Abs(*d)
		if err != nil {
			return xerrors.Errorf("could not resolve path %q", *d)
		}
		if err = os.MkdirAll(v, 0755); err != nil {
			return xerrors.Errorf("could not create directory %s: %w", v, err)
		}
		v, err = realpath.Realpath(v)
		if err != nil {
			return xerrors.Errorf("could not determine realpath for %q", *d)
		}
		*d = v
	}

	return nil
}

// checkNode checks that node is available and the correct version.
//
// If node is not available, then the latest version is downloaded to the cache
// dir and used instead.
func checkNode(flags *Flags) error {
	var err error

	if flags.Node == "" {
		if flags.Node, flags.NodeBin, err = installNode(flags); err != nil {
			return err
		}
	}

	node, err := realpath.Realpath(flags.Node)
	if err != nil {
		return err
	}
	flags.Node = node

	if flags.NodeBin == "" {
		if runtime.GOOS == "windows" {
			flags.NodeBin = filepath.Join(flags.Node, "node.exe")
		} else {
			flags.NodeBin = filepath.Join(flags.Node, "bin", "node")
		}
	}

	// check node version
	nodeVer, err := runCombined(flags, flags.NodeBin, "--version")
	if err != nil {
		return xerrors.Errorf("unable to determine node version: %w", err)
	}
	if !compareSemver(nodeVer, nodeConstraint) {
		return xerrors.Errorf("%s version must be %s, currently: %s", flags.NodeBin, nodeConstraint, nodeVer)
	}

	return nil
}

// checkYarn checks that yarn is available and the correct version.
//
// If yarn is not available, then the latest version is downloaded to the cache
// dir and used instead.
func checkYarn(flags *Flags) error {
	var err error

	if flags.Yarn == "" {
		if flags.Yarn, flags.YarnBin, err = installYarn(flags); err != nil {
			return err
		}
	}

	yarn, err := realpath.Realpath(flags.Yarn)
	if err != nil {
		return err
	}
	flags.Yarn = yarn

	if flags.YarnBin == "" {
		if runtime.GOOS == "windows" {
			flags.YarnBin = filepath.Join(flags.Yarn, "bin", "yarn.cmd")
		} else {
			flags.YarnBin = filepath.Join(flags.Yarn, "bin", "yarn")
		}
	}

	// check yarn version
	yarnVer, err := runCombined(flags, flags.YarnBin, "--version")
	if err != nil {
		return xerrors.Errorf("unable to determine yarn version: %w", err)
	}
	if !compareSemver(strings.TrimPrefix(yarnVer, "v"), yarnConstraint) {
		return xerrors.Errorf("%s version must be %s, currently: %s", flags.YarnBin, yarnConstraint, yarnVer)
	}
	return nil
}
