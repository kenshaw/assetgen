package gen

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/kenshaw/assetgen/pack"
	"github.com/yookoala/realpath"
)

const (
	nodeConstraint    = ">=14.16.x"
	yarnConstraint    = ">=1.22.x"
	cacheDir          = ".cache"
	buildDir          = "build"
	nodeModulesDir    = "node_modules"
	nodeModulesBinDir = ".bin"
	assetsDir         = "assets"
	productionEnv     = "production"
	developmentEnv    = "development"
	distDir           = "dist"
	scriptName        = "assets.anko"
	assetsFile        = "assets.go"
	fontsDir          = "fonts"
	imagesDir         = "images"
	jsDir             = "js"
	sassDir           = "sass"
	cssDir            = "css"
	sassJs            = "sass.js"
	assetgenScss      = "_assetgen.scss"
	templatesDir      = "templates"
	nodeDistURL       = "https://nodejs.org/dist"
)

// Run generates assets using the current working directory and default flags.
func Run() error {
	// load working directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine working directory: %w", err)
	}
	// build flags
	flags := NewFlags(wd)
	fs := flags.FlagSet(filepath.Base(os.Args[0]), flag.ExitOnError)
	if err := fs.Parse(os.Args[1:]); err != nil {
		return fmt.Errorf("could not parse args: %w", err)
	}
	return Assetgen(flags)
}

// Assetgen generates assets based on the passed flags.
func Assetgen(flags *Flags) error {
	// check working directory is usable
	wdfi, err := os.Stat(flags.Wd)
	if err != nil || !wdfi.IsDir() {
		return fmt.Errorf("cannot read from working directory %q", flags.Wd)
	}
	wd, err := realpath.Realpath(flags.Wd)
	if err != nil {
		return fmt.Errorf("could not determine real path for %s: %w", flags.Wd, err)
	}
	flags.Wd = wd
	// ensure workers is at least 1
	if flags.Workers < 1 {
		return errors.New("workers must be at least 1")
	}
	// ensure valid trans func name
	if !isValidIdentifier(flags.TFuncName) {
		return errors.New("invalid trans func name")
	}
	// ensure paths are set
	if flags.Cache == "" {
		if dir := os.Getenv("ASSETGEN_CACHE"); dir != "" {
			flags.Cache = dir
		} else {
			flags.Cache = filepath.Join(flags.Wd, cacheDir)
		}
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
	if flags.Dist == "" {
		flags.Dist = filepath.Join(flags.Assets, distDir)
	}
	if flags.Script == "" {
		flags.Script = filepath.Join(flags.Assets, scriptName)
	}
	// set working directory
	if err := os.Chdir(flags.Wd); err != nil {
		return fmt.Errorf("could not change to dir: %w", err)
	}
	// check setup
	if err := checkSetup(flags); err != nil {
		return err
	}
	// set PATH
	if err := os.Setenv("PATH", strings.Join([]string{
		filepath.Dir(flags.NodeBin),
		flags.NodeModulesBin,
		os.Getenv("PATH"),
	}, ":")); err != nil {
		return fmt.Errorf("could not set PATH: %w", err)
	}
	// set NODE_PATH
	if err := os.Setenv("NODE_PATH", flags.NodeModules); err != nil {
		return fmt.Errorf("could not set NODE_PATH: %w", err)
	}
	// load script
	s, err := LoadScript(flags)
	if err != nil {
		return fmt.Errorf("unable to load script %s: %w", flags.Script, err)
	}
	// setup dependencies
	if err := s.ConfigDeps(); err != nil {
		return fmt.Errorf("unable to configure dependencies: %w", err)
	}
	// fix links in node/.bin directory
	if err := fixNodeModulesBinLinks(flags); err != nil {
		return fmt.Errorf("unable to fix bin links in %s: %w", flags.NodeModulesBin, err)
	}
	// recreate dist
	if err := os.RemoveAll(s.flags.Dist); err != nil {
		return fmt.Errorf("unable to remove %s: %w", s.flags.Dist, err)
	}
	if err := os.MkdirAll(s.flags.Dist, 0755); err != nil {
		return fmt.Errorf("unable to create %s: %w", s.flags.Dist, err)
	}
	dist, err := pack.NewBase(s.flags.Dist, pack.WithManifest(s.flags.PackManifest))
	if err != nil {
		return fmt.Errorf("unable to create dist: %w", err)
	}
	ctxt, cancel := context.WithCancel(context.Background())
	// start callback server
	sock, err := s.startCallbackServer(ctxt, dist)
	if err != nil {
		return fmt.Errorf("could not start callback server: %w", err)
	}
	defer func() {
		cancel()
		if err := os.RemoveAll(filepath.Dir(sock)); err != nil {
			warnf(flags, "could not remove %s: %w", sock, err)
		}
	}()
	// set ASSETGEN_SOCK
	if err := os.Setenv("ASSETGEN_SOCK", sock); err != nil {
		return fmt.Errorf("could not set ASSETGEN_SOCK: %w", err)
	}
	// run script
	if err := s.Execute(dist); err != nil {
		return fmt.Errorf("could not run script: %w", err)
	}
	// write assets.go
	if err := writeAssetsGo(flags, dist); err != nil {
		return fmt.Errorf("could not write %s: %w", assetsFile, err)
	}
	return nil
}

// checkSetup checks that yarn is the correct version, and all necessary files
// and directories exist as expected.
func checkSetup(flags *Flags) error {
	// ensure primary directories exist
	if err := checkDirs(flags, &flags.Cache, &flags.Build, &flags.Assets, &flags.Dist); err != nil {
		return fmt.Errorf("unable to fix .cache build assets: %w", err)
	}
	// check node + yarn
	if err := checkNode(flags); err != nil {
		return err
	}
	if err := os.Setenv("PATH", filepath.Dir(flags.NodeBin)+":"+os.Getenv("PATH")); err != nil {
		return err
	}
	if err := checkYarn(flags); err != nil {
		return err
	}
	// determine if node_modules and yarn.lock is present
	var nodeModulesPresent, yarnLockPresent bool
	if _, err := os.Stat(flags.NodeModules); err == nil {
		nodeModulesPresent = true
	}
	if _, err := os.Stat(filepath.Join(flags.Wd, "yarn.lock")); err == nil {
		yarnLockPresent = true
	}
	// check dirs node_modules + node_modules/.bin
	if err := checkDirs(flags, &flags.NodeModules, &flags.NodeModulesBin); err != nil {
		return fmt.Errorf("unable to fix node_modules and node_modules/.bin: %w", err)
	}
	// setup files
	if err := setupFiles(flags); err != nil {
		return fmt.Errorf("unable to setup files: %w", err)
	}
	// do pure lockfile install
	if !nodeModulesPresent && yarnLockPresent {
		if err := run(flags, flags.YarnBin, "install", "--pure-lockfile", "--no-bin-links", "--modules-folder="+flags.NodeModules); err != nil {
			return errors.New("unable to install locked deps: please fix manually")
		}
	}
	// ensure assets and dist directories exists
	for _, d := range []struct{ n, v string }{
		{"assets", flags.Assets},
	} {
		_, err := filepath.Rel(flags.Wd, d.v)
		if err != nil || !isParentDir(flags.Wd, d.v) {
			return fmt.Errorf("%s path must be subdirectory of working directory", d.n)
		}
	}
	for _, d := range []struct{ n, v string }{
		{"dist", flags.Dist},
	} {
		_, err := filepath.Rel(flags.Assets, d.v)
		if err != nil || !isParentDir(flags.Assets, d.v) {
			return fmt.Errorf("%s path must be subdirectory of assets directory", d.n)
		}
	}
	// run yarn install
	if err := runSilent(flags, flags.YarnBin, "install", "--no-bin-links", "--modules-folder="+flags.NodeModules); err != nil {
		return errors.New("yarn is out of sync: please fix manually")
	}
	// run yarn upgrade
	if flags.YarnUpgrade {
		params := []string{"upgrade", "--no-bin-links", "--modules-folder=" + flags.NodeModules}
		if flags.YarnLatest {
			params = append(params, "--latest")
		}
		if err := runSilent(flags, flags.YarnBin, params...); err != nil {
			return fmt.Errorf("unable to run yarn upgrade: %w", err)
		}
	}
	return nil
}

// checkDirs creates required directories and ensures directories are
// subdirectories of the working directory.
func checkDirs(flags *Flags, dirs ...*string) error {
	// make required directories
	for _, d := range dirs {
		v, err := filepath.Abs(*d)
		if err != nil {
			return fmt.Errorf("could not resolve path %q", *d)
		}
		if err := os.MkdirAll(v, 0755); err != nil {
			return fmt.Errorf("could not create directory %s: %w", v, err)
		}
		if v, err = realpath.Realpath(v); err != nil {
			return fmt.Errorf("could not determine realpath for %q", *d)
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
	if flags.Node == "" {
		var err error
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
		return fmt.Errorf("unable to determine node version: %w", err)
	}
	if !compareSemver(nodeVer, nodeConstraint) {
		return fmt.Errorf("%s version must be %s, currently: %s", flags.NodeBin, nodeConstraint, nodeVer)
	}
	return nil
}

// checkYarn checks that yarn is available and the correct version.
//
// If yarn is not available, then the latest version is downloaded to the cache
// dir and used instead.
func checkYarn(flags *Flags) error {
	if flags.Yarn == "" {
		var err error
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
		return fmt.Errorf("unable to determine yarn version: %w", err)
	}
	if !compareSemver(strings.TrimPrefix(yarnVer, "v"), yarnConstraint) {
		return fmt.Errorf("%s version must be %s, currently: %s", flags.YarnBin, yarnConstraint, yarnVer)
	}
	return nil
}
