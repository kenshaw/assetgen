package gen

import (
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// setupFiles creates default files when they do not already exist.
func setupFiles(flags *Flags) error {
	var err error

	// build relative node path
	nodepath, err := filepath.Rel(flags.Wd, flags.Node)
	if err != nil || !isParentDir(flags.Wd, flags.Node) {
		return errors.New("node path must be subdirectory of working directory")
	}
	app := filepath.Base(flags.Wd)

	// build relative cache paths
	var cacheList string
	for i, d := range buildCacheDirs(flags.Wd, flags.Cache, flags.Node, flags.NodeBin) {
		if i != 0 {
			cacheList += ","
		}
		cacheList = cacheList + fmt.Sprintf("\n    %q", d)
	}

	// create files if not present
	for _, d := range []struct{ path, contents string }{
		{filepath.Join(flags.Wd, yarnRcName), fmt.Sprintf(yarnRcTemplate, nodepath)},
		{filepath.Join(flags.Wd, "package.json"), fmt.Sprintf(packageJsonTemplate, app, app+" app", cacheList)},
		{filepath.Join(flags.Assets, ".gitignore"), assetsGitignoreTemplate},
		{filepath.Join(flags.Assets, scriptName), scriptTemplate},
	} {
		err = writeCond(d.path, d.contents)
		if err != nil {
			return fmt.Errorf("unable to setup %s: %v", d.path, err)
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

const (
	// yarnRcTemplate is the default $CWD/.yarnrc contents.
	yarnRcTemplate = `--modules-folder %q
--*.no-bin-links true`

	// packageJsonTemplate is the default $CWD/package.json contents.
	packageJsonTemplate = `{
  "name": %q,
  "description": %q,
  "license": "UNLICENSED",
  "private": true,
  "cacheDirectories": [%s
  ],
  "dependencies": {}
}`

	// scriptTemplate is the default $ASSETS/assets.anko contents.
	scriptTemplate = `# generated placeholder script 

# js("js/app.js", ...)
# sass("css/app.css", ...)`

	// assetsGitignoreTemplate is the default $ASSETS/.gitignore contents.
	assetsGitignoreTemplate = `/assets.go
/locales/locales.go
/geoip/*.gz
*.html.go
*.mo`
)
