package binpack

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// byName satisfies sort.Interface to sort []os.FileInfo by name.
type byName []os.FileInfo

func (v byName) Len() int           { return len(v) }
func (v byName) Swap(i, j int)      { v[i], v[j] = v[j], v[i] }
func (v byName) Less(i, j int) bool { return v[i].Name() < v[j].Name() }

// findFiles recursively finds all the file paths in the given directory tree.
// They are added to the given map as keys. Values will be safe function names
// for each file, which will be used when generating the output code.
func findFiles(dir, prefix string, recursive bool, toc *[]Asset, ignore []*regexp.Regexp, knownFuncs map[string]int, visitedPaths map[string]bool) error {
	dirpath := dir
	if len(prefix) > 0 {
		dirpath, _ = filepath.Abs(dirpath)
		prefix, _ = filepath.Abs(prefix)
		prefix = filepath.ToSlash(prefix)
	}

	fi, err := os.Stat(dirpath)
	if err != nil {
		return err
	}

	var list []os.FileInfo

	if !fi.IsDir() {
		dirpath = filepath.Dir(dirpath)
		list = []os.FileInfo{fi}
	} else {
		visitedPaths[dirpath] = true
		fd, err := os.Open(dirpath)
		if err != nil {
			return err
		}

		defer fd.Close()

		list, err = fd.Readdir(0)
		if err != nil {
			return err
		}

		// sort to make output stable between invocations
		sort.Sort(byName(list))
	}

	for _, file := range list {
		var asset Asset
		asset.Path = filepath.Join(dirpath, file.Name())
		asset.Name = filepath.ToSlash(asset.Path)

		var ignoring bool
		for _, re := range ignore {
			if re.MatchString(asset.Path) {
				ignoring = true
				break
			}
		}
		if ignoring {
			continue
		}

		if file.IsDir() {
			if recursive {
				recursivePath := filepath.Join(dir, file.Name())
				visitedPaths[asset.Path] = true
				findFiles(recursivePath, prefix, recursive, toc, ignore, knownFuncs, visitedPaths)
			}
			continue
		} else if file.Mode()&os.ModeSymlink == os.ModeSymlink {
			var linkPath string
			if linkPath, err = os.Readlink(asset.Path); err != nil {
				return err
			}
			if !filepath.IsAbs(linkPath) {
				if linkPath, err = filepath.Abs(dirpath + "/" + linkPath); err != nil {
					return err
				}
			}
			if _, ok := visitedPaths[linkPath]; !ok {
				visitedPaths[linkPath] = true
				findFiles(asset.Path, prefix, recursive, toc, ignore, knownFuncs, visitedPaths)
			}
			continue
		}

		if strings.HasPrefix(asset.Name, prefix) {
			asset.Name = asset.Name[len(prefix):]
		} else {
			asset.Name = filepath.Join(dir, file.Name())
		}

		// if we have a leading slash, get rid of it
		if len(asset.Name) > 0 && asset.Name[0] == '/' {
			asset.Name = asset.Name[1:]
		}

		// this shouldn't happen
		if len(asset.Name) == 0 {
			return fmt.Errorf("invalid file: %v", asset.Path)
		}

		asset.Func = safeFuncName(asset.Name, knownFuncs)
		asset.Path, _ = filepath.Abs(asset.Path)
		*toc = append(*toc, asset)
	}

	return nil
}
