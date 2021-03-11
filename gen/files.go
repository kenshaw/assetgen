package gen

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kenshaw/assetgen/pack"
)

// setupFiles creates default files when they do not already exist.
func setupFiles(flags *Flags) error {
	app := filepath.Base(flags.Wd)
	// build relative cache paths
	var cacheList string
	for i, d := range buildCacheDirs(flags.Wd, flags.Cache, flags.NodeModules, flags.NodeModulesBin) {
		if i != 0 {
			cacheList += ","
		}
		cacheList = cacheList + fmt.Sprintf("\n    %q", d)
	}
	// create files if not present
	for _, d := range []struct{ path, contents string }{
		{filepath.Join(flags.Wd, "package.json"), tplf("package.json", app, app+" app", cacheList)},
		{filepath.Join(flags.Assets, ".gitignore"), tplf("gitignore")},
		{filepath.Join(flags.Assets, scriptName), tplf("assets.anko")},
	} {
		if err := writeCond(d.path, d.contents); err != nil {
			return fmt.Errorf("unable to setup %s: %w", d.path, err)
		}
	}
	return nil
}

// buildCacheDirs builds a list of directory paths relative to the working
// directory wd to cache.
//
// Only directories that are relative to wd and not previously cached by an
// earlier path will be returned.
func buildCacheDirs(wd string, paths ...string) []string {
	type dir struct {
		dir, rel string
		add      bool
	}
	// determine which of the supplied paths are children of wd
	var dirs []dir
	for _, p := range paths {
		if r, err := filepath.Rel(wd, p); err == nil {
			dirs = append(dirs, dir{p, r, true})
		}
	}
	// build list
	var d []string
	for i := len(dirs) - 1; i >= 0; i-- {
		// work from end, only add dirs where there no earlier dir is a parent
		// (and thus already cached)
		for j := len(dirs) - 1; j >= 0 && dirs[i].add; j-- {
			if j == i {
				continue
			}
			if isParentDir(dirs[j].dir, dirs[i].dir) {
				dirs[i].add = false
				break
			}
		}
		if dirs[i].add {
			d = append(d, dirs[i].rel)
		}
	}
	sort.Strings(d)
	return d
}

// writeCond conditionally writes contents to path if path doesn't exist.
//
// Note: never writes a blank file: always adds \n if not present in contents.
func writeCond(path, contents string) error {
	fi, err := os.Stat(path)
	switch {
	case err != nil && os.IsNotExist(err):
		return ioutil.WriteFile(path, []byte(strings.TrimSuffix(contents, "\n")+"\n"), 0644)
	case err != nil:
		return err
	case fi.IsDir():
		return errors.New("must not be a directory")
	}
	return nil
}

// writeAssetsGo generates the assets.go for the packed assets.
func writeAssetsGo(flags *Flags, dist *pack.Pack) error {
	// write manifest
	if err := dist.WriteManifestInverted(); err != nil {
		return fmt.Errorf("unable to write manifest: %w", err)
	}
	distshort := strings.TrimPrefix(flags.Dist, flags.Assets+"/")
	// build asset list
	manifest, err := dist.Manifest()
	if err != nil {
		return fmt.Errorf("unable to load manifest: %w", err)
	}
	var assets []string
	for k := range manifest {
		assets = append(assets, k)
	}
	sort.Strings(assets)
	for i := 0; i < len(assets); i++ {
		assets[i] = `//go:embed ` + path.Join(distshort, assets[i])
	}
	assets = append([]string{`//go:embed ` + path.Join(distshort, flags.PackManifest)}, assets...)
	// write assets.go
	return ioutil.WriteFile(
		filepath.Join(flags.Assets, assetsFile),
		[]byte(tplf(assetsFile, strings.Join(assets, "\n"), distshort, flags.PackManifest)),
		0644,
	)
}
