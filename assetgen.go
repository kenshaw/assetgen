package assetgen

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/yookoala/realpath"
)

const (
	yarnConstraint = ">=1.12.x, <1.13"
	yarnRcName     = ".yarnrc"

	cacheDir   = ".cache"
	buildDir   = "build"
	distDir    = "dist"
	nodeDir    = "node_modules"
	nodeBinDir = ".bin"
	assetsDir  = "assets"

	productionEnv  = "production"
	developmentEnv = "development"

	scriptName   = "assets.anko"
	fontsDir     = "fonts"
	geoipDir     = "geoip"
	localesDir   = "locales"
	imagesDir    = "images"
	jsDir        = "js"
	sassDir      = "sass"
	templatesDir = "templates"
)

// Run generates assets using the current working directory and default flags.
func Run() error {
	var err error

	// load working directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine working directory: %v", err)
	}

	// build flags
	flags := NewFlags(wd)
	fs := flags.FlagSet(filepath.Base(os.Args[0]), flag.ExitOnError)
	err = fs.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("could not parse args: %v", err)
	}

	return Assetgen(flags)
}

// Assetgen generates assets based on the passed flags.
func Assetgen(flags *Flags) error {
	var err error

	// check working directory is usable
	wdfi, err := os.Stat(flags.Wd)
	if err != nil || !wdfi.IsDir() {
		return fmt.Errorf("cannot read from working directory %q", flags.Wd)
	}
	wd, err := realpath.Realpath(flags.Wd)
	if err != nil {
		return fmt.Errorf("could not determine real path for %s: %v", flags.Wd, err)
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
		flags.Cache = filepath.Join(flags.Wd, cacheDir)
	}
	if flags.Build == "" {
		flags.Build = filepath.Join(flags.Wd, buildDir)
	}
	if flags.Dist == "" {
		flags.Dist = filepath.Join(flags.Build, distDir)
	}
	if flags.Node == "" {
		flags.Node = filepath.Join(flags.Cache, nodeDir)
	}
	if flags.NodeBin == "" {
		flags.NodeBin = filepath.Join(flags.Node, nodeBinDir)
	}
	if flags.Assets == "" {
		flags.Assets = filepath.Join(flags.Wd, assetsDir)
	}
	if flags.Script == "" {
		flags.Script = filepath.Join(flags.Assets, scriptName)
	}

	/*if flags.Env == "" {
		flags.Env = developmentEnv
	}
	*/
	// force noupdate when env == production
	/*if flags.Env == productionEnv {
		flags.NoUpdate = true
	}*/

	// set working directory
	err = os.Chdir(flags.Wd)
	if err != nil {
		return fmt.Errorf("could not change to dir: %v", err)
	}

	// set PATH
	err = os.Setenv("PATH", flags.NodeBin+":"+os.Getenv("PATH"))
	if err != nil {
		return fmt.Errorf("could not set PATH: %v", err)
	}

	// resolve yarn location
	if flags.Yarn == "" {
		flags.Yarn, err = exec.LookPath("yarn")
		if err != nil {
			return errors.New("yarn executable not in PATH")
		}
	}

	// check setupt
	err = checkSetup(flags)
	if err != nil {
		return err
	}

	// load script
	s, err := LoadScript(flags)
	if err != nil {
		return fmt.Errorf("unable to load script %s: %v", flags.Script, err)
	}

	// setup dependencies
	err = s.ConfigDeps()
	if err != nil {
		return fmt.Errorf("unable to configure dependencies: %v", err)
	}

	// fix links in node/.bin directory
	err = fixNodeBinLinks(flags)
	if err != nil {
		return fmt.Errorf("unable to fix bin links in %s: %v", flags.NodeBin, err)
	}

	// run script
	err = s.Execute()
	if err != nil {
		return fmt.Errorf("could not run script: %v", err)
	}

	return nil
}

// checkSetup checks that yarn is the correct version, and all necessary files
// and directories exist as expected.
func checkSetup(flags *Flags) error {
	// check yarn version
	yarnVer, err := runCombined(flags, flags.Yarn, "--version")
	if err != nil {
		return fmt.Errorf("unable to determine yarn version: %v", err)
	}
	if !compareSemver(strings.TrimPrefix(yarnVer, "v"), yarnConstraint) {
		return fmt.Errorf("%s version must be %s, currently: %s", flags.Yarn, yarnConstraint, yarnVer)
	}

	// ensure primary directories exist
	err = checkDirs(flags)
	if err != nil {
		return fmt.Errorf("could not create directories: %v", err)
	}

	// setup files
	err = setupFiles(flags)
	if err != nil {
		return fmt.Errorf("unable to setup files: %v", err)
	}

	// run yarn check
	err = runSilent(flags, flags.Yarn, "check")
	if err != nil {
		return errors.New("yarn is out of sync: please run yarn manually")
	}

	return nil
}

// checkDirs creates required directories and ensures node and assets are
// subdirectories of the working directory.
func checkDirs(flags *Flags) error {
	// make required directories
	for _, d := range []*string{&flags.Cache, &flags.Build, &flags.Dist, &flags.Node, &flags.NodeBin, &flags.Assets} {
		v, err := filepath.Abs(*d)
		if err != nil {
			return fmt.Errorf("could not resolve path %q", *d)
		}
		err = os.MkdirAll(v, 0755)
		if err != nil {
			return fmt.Errorf("could not create directory %s: %v", v, err)
		}
		v, err = realpath.Realpath(v)
		if err != nil {
			return fmt.Errorf("could not determine realpath for %q", *d)
		}
		*d = v
	}

	// ensure node and assets directories exist
	for _, d := range []struct{ n, v string }{
		{"node", flags.Node},
		{"assets", flags.Assets},
	} {
		_, err := filepath.Rel(flags.Wd, d.v)
		if err != nil || !isParentDir(flags.Wd, d.v) {
			return fmt.Errorf("%s path must be subdirectory of working directory", d.n)
		}
	}

	return nil
}

// fixNodeBinLinks walks all packages in flags.Node, reading their bin entries from
// package.json, and creating the appropriate symlink in flags.NodeBin.
func fixNodeBinLinks(flags *Flags) error {
	// check dirs again
	err := checkDirs(flags)
	if err != nil {
		return fmt.Errorf("unable to fix node bin directory: %v", err)
	}

	// erase all links in bin dir
	err = filepath.Walk(flags.NodeBin, func(path string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case path == flags.NodeBin:
			return nil
		case fi.Mode()&os.ModeSymlink == 0:
			return fmt.Errorf("%s is not a symlink", path)
		}
		err = os.Remove(path)
		if err != nil {
			return fmt.Errorf("unable to remove %s: %v", path, err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// grab all bin links defined in package.json
	type link struct {
		dir, path string
	}
	links := make(map[string][]link)
	err = filepath.Walk(flags.Node, func(path string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.IsDir() || filepath.Base(path) != "package.json" {
			return nil
		}

		// decode package.json
		buf, err := ioutil.ReadFile(path)
		if err != nil {
			return err
		}
		var pkgDesc struct {
			Name string      `json:"name"`
			Bin  interface{} `json:"bin"`
		}
		err = json.Unmarshal(buf, &pkgDesc)
		if err != nil {
			warnf(flags, "could not unmarshal %s: %v", path, err)
			return nil
		}
		if pkgDesc.Bin == nil {
			return nil
		}

		// add to links
		pathDir := filepath.Dir(path)
		for n, v := range forceMap(pkgDesc.Bin, pkgDesc.Name, filepath.Base(pathDir)) {
			links[n] = append(links[n], link{
				dir:  pathDir,
				path: v,
			})
		}

		return nil
	})
	if err != nil {
		return err
	}

	// process links
	for n, v := range links {
		l := v[0]

		// determine "most appropriate" link
		for _, z := range v {
			rel, err := filepath.Rel(flags.Node, z.dir)
			if err != nil {
				return fmt.Errorf("could not determine node-relative path for %s: %v", z.dir, err)
			}
			if !strings.Contains(rel, string(filepath.Separator)) {
				l = z
				break
			}
		}

		// create symlink
		linkpath := filepath.Join(l.dir, l.path)
		oldname, err := realpath.Realpath(linkpath)
		if err != nil {
			return fmt.Errorf("unable to determine path for %s: %v", linkpath, err)
		}
		newname := filepath.Join(flags.NodeBin, n)

		// check symlink exists
		_, err = os.Stat(newname)
		switch {
		case err != nil && os.IsNotExist(err):
		case err != nil:
			return err
		}

		// symlink
		err = os.Symlink(oldname, newname)
		if err != nil {
			return fmt.Errorf("unable to symlink %s to %s: %v", newname, oldname, err)
		}

		// fix permissions
		if runtime.GOOS != "windows" {
			err = os.Chmod(linkpath, 0755)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
