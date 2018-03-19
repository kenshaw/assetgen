package assetgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/mattn/anko/vm"
)

// dep wraps package dependency information.
type dep struct {
	name string
	ver  string
}

// Script wraps an assetgen script.
type Script struct {
	flags *Flags

	// deps are package dependencies.
	nodeDeps []dep

	// sassIncludes are sass include dependencies.
	sassIncludes []dep

	// pre are the pre setup steps to be executed in order.
	pre []func() error

	// exec is the steps to be executed, in order.
	exec []func() error

	// post are the post setup steps to be executed in order.
	post []func() error
}

// concat is the script handler to concat one or more files.
func (s *Script) concat(params ...interface{}) {
	s.exec = append(s.exec, func() error {
		return nil
	})
}

// js is the script handler to generate a minified javascript file from one or
// more files.
func (s *Script) js(params ...interface{}) {
	s.exec = append(s.exec, func() error {
		return nil
	})
}

// LoadScript loads an assetgen script using the specified flags.
func LoadScript(flags *Flags) (*Script, error) {
	var err error

	// load
	buf, err := ioutil.ReadFile(flags.Script)
	if err != nil {
		return nil, fmt.Errorf("unable to load script %s: %v", flags.Script, err)
	}

	// create
	s := &Script{
		flags: flags,
	}

	// create scripting runtime
	a := vm.NewEnv()

	// add flags
	err = a.Define("flags", flags)

	// define vals
	for _, z := range []struct {
		n string
		v interface{}
	}{
		{"flags", flags},
		{"build", flags.Build},
		{"cache", flags.Cache},
		{"node", flags.Node},
	} {
		err = a.Define(z.n, z.v)
		if err != nil {
			return nil, fmt.Errorf("unable to define %s: %v", z.n, err)
		}
	}

	// execute
	_, err = a.Execute(string(buf))
	if err != nil {
		return nil, fmt.Errorf("unable to execute script %s: %v", flags.Script, err)
	}

	// add directory handling steps
	for _, d := range []struct {
		n string
		f func(string, string)
	}{
		{"fonts", s.addFonts},
		{"geoip", s.addGeoip},
		{"images", s.addImages},
		{"locales", s.addLocales},
		{"templates", s.addTemplates},
	} {
		// skip adding step if directory not present
		dir := filepath.Join(flags.Assets, d.n)
		fi, err := os.Stat(dir)
		switch {
		case err != nil && os.IsNotExist(err):
			continue
		case err != nil:
			return nil, fmt.Errorf("could not stat %s: %v", dir, err)
		case !fi.IsDir():
			return nil, fmt.Errorf("path %s must be a directory", dir)
		}
		d.f(d.n, dir)
	}

	return s, nil
}

// addFonts configures a script step for packing static font files.
//
// This walks the fonts directory, and if there's a SCSS/CSS file, add it to
// sass import path. All font files will be added to the manifest.
func (s *Script) addFonts(n, dir string) {
}

// addGeoip configures a script step for packing geoip data.
//
// This looks at the geoip directory, and if there is no geoip data, downloads
// the appropriate file (if it doesn't exist), and adds the file to the packed
// data (but not to the manifest).
func (s *Script) addGeoip(n, dir string) {

}

// addImages configures a script step for optimizing and packing image files.
//
// This walks the images directory, and if there's any image files, generates
// the optimized image (in the cache directory, along with a md5 content hash
// of the original image) and adds the optimized image to the manifest.
//
// Note: adds the appropriate dependency requirements to script's deps.
func (s *Script) addImages(n, dir string) {
	for _, n := range []string{
		"imagemin-cli",
		"imagemin-gifsicle",
		"imagemin-guetzli",
		"imagemin-pngquant",
		"imagemin-svgo",
	} {
		s.nodeDeps = append(s.nodeDeps, dep{n, ""})
	}
}

// addLocales configures a script step for packing locales data.
//
// This looks at the locales directory, and if there is any locales adds them
// to the packed data (but not to the manifest).
func (s *Script) addLocales(n, dir string) {

}

// addTemplates configures a script step for generating optimized template
// output (ie, Go code) from quicktemplate'd HTML files.
//
// This looks at the templates directory, and if there are any .html files,
// minifies them and normalizes templated i18n translation calls (T) before
// passing the template through the quicktemplate compiler (qtc).
//
// Note :adds the appropriate dependency requirements to script's deps.
func (s *Script) addTemplates(n, dir string) {

}

// ConfigDeps handles configuring dependencies.
func (s *Script) ConfigDeps() error {
	// load package.json
	buf, err := ioutil.ReadFile(filepath.Join(s.flags.Wd, "package.json"))
	if err != nil {
		return err
	}

	var v struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	err = json.Unmarshal(buf, &v)
	if err != nil {
		return errors.New("invalid package.json")
	}

	// build params
	params := []string{"add", "--no-progress", "--silent"}
	var add bool
	for _, d := range s.nodeDeps {
		if _, ok := v.Dependencies[d.name]; ok {
			continue
		}
		pkg := d.name
		if d.ver != "" {
			pkg += "@" + d.ver
		}
		params, add = append(params, pkg), true
	}
	if !add {
		return nil
	}

	return run(s.flags, s.flags.Yarn, params...)
}

// Execute executes the script.
func (s *Script) Execute() error {
	return nil
}
