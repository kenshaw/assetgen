package gen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/gobwas/glob"
	"github.com/kenshaw/assetgen/pack"
	"github.com/mattn/anko/env"
	"github.com/mattn/anko/vm"
	qtcparser "github.com/valyala/quicktemplate/parser"
	"github.com/yookoala/realpath"
	"golang.org/x/sync/errgroup"
)

// dep wraps package dependency information.
type dep struct {
	name string
	ver  string
}

// jsdep wraps js dependency information.
type jsdep struct {
	name string
	ver  string
	path string
}

// Script wraps an assetgen script.
type Script struct {
	flags *Flags
	// logf is the log func.
	logf func(string, ...interface{})
	// nodeDeps are node package dependencies.
	nodeDeps []dep
	// sassIncludes are sass include directories.
	sassIncludes []string
	// pre are the pre setup steps to be executed in order.
	pre []func() error
	// exec is the steps to be executed, in order.
	exec []func(*pack.Pack) error
	// post are the post setup steps to be executed in order.
	post []func() error
}

// LoadScript loads an assetgen script using the specified flags.
func LoadScript(flags *Flags) (*Script, error) {
	// load
	buf, err := ioutil.ReadFile(flags.Script)
	if err != nil {
		return nil, fmt.Errorf("unable to load script %s: %w", flags.Script, err)
	}
	// create
	s := &Script{
		flags: flags,
		logf:  log.Printf,
	}
	// create scripting runtime
	a := env.NewEnv()
	// define vals
	for _, z := range []struct {
		n string
		v interface{}
	}{
		{"staticDir", s.staticDir},
		{"sassIncludeNodeModules", s.sassIncludeNodeModules},
		{"sassInclude", s.sassInclude},
		{"npmjs", s.npmjs},
		{"js", s.js},
	} {
		if err := a.Define(z.n, z.v); err != nil {
			return nil, fmt.Errorf("unable to define %s: %w", z.n, err)
		}
	}
	// execute
	if _, err := vm.Execute(a, nil, string(buf)); err != nil {
		return nil, fmt.Errorf("unable to execute script %s: %w", flags.Script, err)
	}
	// add directory handling steps
	for _, d := range []struct {
		n string
		f func(string, string)
	}{
		{"fonts", s.addFonts},
		{"images", s.addImages},
		{"sass", s.addSass},
		{"templates", s.addTemplates},
	} {
		// skip adding step if directory not present
		dir := filepath.Join(flags.Assets, d.n)
		fi, err := os.Stat(dir)
		switch {
		case err != nil && os.IsNotExist(err):
			continue
		case err != nil:
			return nil, fmt.Errorf("could not stat %s: %w", dir, err)
		case !fi.IsDir():
			return nil, fmt.Errorf("path %s must be a directory", dir)
		}
		d.f(d.n, dir)
	}
	return s, nil
}

// get retrieves src.
func (s *Script) get(src string) ([]byte, error) {
	res, err := http.Get(src)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve %q: %w", src, err)
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}

// concat is the script handler to concat one or more files.
func (s *Script) concat(params ...interface{}) {
	s.exec = append(s.exec, func(dist *pack.Pack) error {
		return nil
	})
}

// npmjs is the script handler that wraps a npm js include.
func (s *Script) npmjs(name string, v ...string) jsdep {
	var ver, path string
	if i := strings.Index(name, "@"); i != -1 {
		ver, name = name[i+1:], name[:i]
	}
	if len(v) != 0 {
		path = v[0]
	}
	return jsdep{
		name: name,
		ver:  ver,
		path: path,
	}
}

var staticDirNameRE = regexp.MustCompile("^[A-Za-z0-9]+$")

// staticDir adds a static directory to the assets.
func (s *Script) staticDir(name string) {
	s.exec = append(s.exec, func(dist *pack.Pack) error {
		if !staticDirNameRE.MatchString(name) {
			return fmt.Errorf("invalid static dir name %q", name)
		}
		dir := filepath.Join(s.flags.Assets, name)
		fi, err := os.Stat(dir)
		switch {
		case err != nil:
			return fmt.Errorf("could not open static dir %q", dir)
		case !fi.IsDir():
			return fmt.Errorf("%q is not a directory", dir)
		}
		return filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
			switch {
			case err != nil:
				return err
			case fi.IsDir():
				return nil
			}
			p, err := filepath.Rel(s.flags.Assets, n)
			if err != nil {
				return fmt.Errorf("%q not located within the project: %w", fi.Name(), err)
			}
			return dist.PackFile(p, n)
		})
	})
}

// sassIncludeNodeModules adds the node modules path to the sass include search
// path.
func (s *Script) sassIncludeNodeModules() {
	s.sassIncludes = append(s.sassIncludes, s.flags.NodeModules)
}

// sassInclude adds a include path for a node module.
func (s *Script) sassInclude(name string, paths ...string) {
	var ver string
	if i := strings.Index(name, "@"); i != -1 {
		ver, name = name[i+1:], name[:i]
	}
	s.nodeDeps = append(s.nodeDeps, dep{name, ver})
	if len(paths) == 0 {
		paths = append(paths, "")
	}
	for _, p := range paths {
		s.sassIncludes = append(s.sassIncludes, filepath.Join(s.flags.NodeModules, name, p))
	}
}

// js is the script handler to generate a minified javascript file from one or
// more files.
func (s *Script) js(fn string, v ...interface{}) {
	for _, n := range []string{
		"uglify-js",
		"source-map",
	} {
		s.nodeDeps = append(s.nodeDeps, dep{n, ""})
	}
	// add node deps
	for _, x := range v {
		switch d := x.(type) {
		case jsdep:
			s.nodeDeps = append(s.nodeDeps, dep{d.name, d.ver})
		}
	}
	s.exec = append(s.exec, func(dist *pack.Pack) error {
		if len(v) < 1 {
			return errors.New("js() must be passed at least one arg")
		}
		// process node deps
		scripts := make([]jsdep, len(v))
		for i := 0; i < len(v); i++ {
			switch d := v[i].(type) {
			case string:
				n := filepath.Join(s.flags.Assets, jsDir, d)
				_, err := os.Stat(n)
				if err != nil {
					return fmt.Errorf("could not find js %q", d)
				}
				scripts[i] = jsdep{path: n}
			case jsdep:
				p, err := s.findNodeModulesFile(d)
				if err != nil {
					return err
				}
				scripts[i] = jsdep{name: d.name, path: p}
			default:
				return fmt.Errorf("unknown type passed to js(): %T", v[i])
			}
		}
		// ensure scripts are contained within project
		for i := 0; i < len(scripts); i++ {
			var err error
			if scripts[i].path, err = filepath.Rel(s.flags.Wd, scripts[i].path); err != nil {
				return fmt.Errorf("js cannot be outside of project")
			}
		}
		// ensure directory exists
		dir := filepath.Join(s.flags.Build, jsDir)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("could not create js dir: %w", err)
		}
		// open out file
		outfile := filepath.Join(dir, fn)
		f, err := os.Create(outfile)
		if err != nil {
			return fmt.Errorf("could not open %q: %w", outfile, err)
		}
		// add all files
		for _, d := range scripts {
			buf, err := ioutil.ReadFile(filepath.Join(s.flags.Wd, d.path))
			if err != nil {
				return fmt.Errorf("could not read js %q: %w", fn, err)
			}
			if _, err := f.WriteString(strings.TrimSuffix(string(buf), "\n") + "\n"); err != nil {
				return fmt.Errorf("could not write %q to %q: %w", fn, outfile, err)
			}
		}
		// close
		if err := f.Close(); err != nil {
			return fmt.Errorf("could not close %q: %w", outfile, err)
		}
		// uglify
		ext := filepath.Ext(outfile)
		uglyfile := strings.TrimSuffix(outfile, ext) + ".uglify" + ext
		if err := run(s.flags,
			"uglifyjs",
			"--source-map",
			"--compress",
			"--output", uglyfile,
			outfile,
		); err != nil {
			return fmt.Errorf("could not uglify %q: %w", outfile, err)
		}
		return dist.PackFile(jsDir+"/"+fn, uglyfile)
	})
}

// addFonts configures a script step for packing static font files.
//
// This walks the fonts directory, and if there's a SCSS/CSS file, add it to
// sass import path. All font files will be added to the manifest.
func (s *Script) addFonts(_, dir string) {
}

var imageExtRE = regexp.MustCompile(`(?i)\.(jpe?g|gif|png|svg|mp4|webm|json)$`)

// addImages configures a script step for optimizing and packing image files.
//
// This walks the images directory, and if there's any image files, generates
// the optimized image (in the cache directory, along with a md5 content hash
// of the original image) and adds the optimized image to the manifest.
//
// Note: adds the appropriate dependency requirements to script's deps.
func (s *Script) addImages(_, dir string) {
	for _, n := range []string{
		"imagemin-cli",
		"imagemin-gifsicle",
		"imagemin-guetzli",
		"imagemin-pngquant",
		"imagemin-svgo",
	} {
		s.nodeDeps = append(s.nodeDeps, dep{n, ""})
	}
	s.exec = append(s.exec, func(dist *pack.Pack) error {
		// accumulate images
		var all, changed []string
		err := filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
			switch {
			case err != nil:
				return err
			case fi.IsDir() || !imageExtRE.MatchString(fi.Name()) || strings.HasPrefix(filepath.Base(n), "."):
				return nil
			}
			// ensure directory exists
			fn := strings.TrimPrefix(n, dir+"/")
			cacheDir := filepath.Join(s.flags.Cache, "images", filepath.Dir(fn))
			if err := os.MkdirAll(cacheDir, 0755); err != nil {
				return err
			}
			outfile := filepath.Join(cacheDir, filepath.Base(fn))
			// hash
			hash, err := md5hash(n)
			if err != nil {
				return err
			}
			hashPath := outfile + ".md5"
			var cached string
			// read cached hash
			_, err = os.Stat(hashPath)
			switch {
			case err != nil && !os.IsNotExist(err):
				return err
			case err != nil && os.IsNotExist(err):
			case err == nil:
				buf, err := ioutil.ReadFile(hashPath)
				if err != nil {
					return err
				}
				cached = string(buf)
			}
			all = append(all, fn)
			if cached == "" || cached != hash || !fileExists(outfile) {
				changed = append(changed, fn)
			}
			return ioutil.WriteFile(hashPath, []byte(hash), 0644)
		})
		if err != nil {
			return err
		}
		ch := make(chan string, len(changed))
		for _, fn := range changed {
			ch <- fn
		}
		close(ch)
		// start workers to optimize images
		eg, ctxt := errgroup.WithContext(context.Background())
		for i := 0; i < s.flags.Workers; i++ {
			eg.Go(func() error {
				for {
					select {
					case <-ctxt.Done():
						return ctxt.Err()
					case fn := <-ch:
						if fn == "" {
							return nil
						}
						out := filepath.Join(s.flags.Cache, "images", fn)
						in := filepath.Join(s.flags.Assets, "images", fn)
						if err := s.optimizeImage(out, in); err != nil {
							return err
						}
					}
				}
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}
		// pack the generated images
		for _, fn := range all {
			if err := dist.PackFile(imagesDir+"/"+fn, filepath.Join(s.flags.Cache, imagesDir, fn)); err != nil {
				return err
			}
		}
		return nil
	})
}

// optimizeImage optimizes a single image.
func (s *Script) optimizeImage(out, in string) error {
	var plugin string
	switch filepath.Ext(strings.ToLower(in))[1:] {
	case "jpg", "jpeg":
		plugin = "--plugin=guetzli"
	case "svg":
		plugin = "--plugin=svgo"
	case "png":
		plugin = "--plugin=pngquant"
	case "gif":
		plugin = "--plugin=gifsicle"
	}
	return runSilent(s.flags, "imagemin", plugin, "--out-dir="+filepath.Dir(out), in)
}

// stripCssCommentsRE is a regexp to match css comments.
var stripCssCommentsRE = regexp.MustCompile(`/\*!.+\*/`)

// addSass configures a script step for compiling and minifying sass assets.
//
// This walks the sass directory, and if there's any .scss files, generates the
// appropriate css after compiling, prefixing, and minifying.
func (s *Script) addSass(_, dir string) {
	for _, n := range []string{
		"autoprefixer",
		"clean-css-cli",
		"deasync",
		"node-sass",
		"tailwindcss",
	} {
		s.nodeDeps = append(s.nodeDeps, dep{n, ""})
	}
	s.exec = append(s.exec, func(dist *pack.Pack) error {
		// ensure build/assetgen exists
		if err := os.MkdirAll(filepath.Join(s.flags.Build, "assetgen"), 0755); err != nil {
			return fmt.Errorf("could not create assetgen directory: %w", err)
		}
		// if tailwind.config.js doesn't exist, generate it
		tailwindJs := filepath.Join(s.flags.Assets, "sass", "tailwind.config.js")
		if !fileExists(tailwindJs) {
			if err := run(s.flags, "tailwindcss", "init", tailwindJs, "--full"); err != nil {
				return fmt.Errorf("could not generate tailwind css config: %w", err)
			}
		}
		// write sass.js and _assetgen.scss to build dir
		if err := ioutil.WriteFile(
			filepath.Join(s.flags.Build, sassJs),
			[]byte(tplf(sassJs)),
			0644,
		); err != nil {
			return fmt.Errorf("could not write %s: %w", sassJs, err)
		}
		if err := ioutil.WriteFile(
			filepath.Join(s.flags.Build, "assetgen", assetgenScss),
			[]byte(tplf(assetgenScss)),
			0644,
		); err != nil {
			return fmt.Errorf("could not write: %s: %w", assetgenScss, err)
		}
		// write fontawesome to build dir
		if err := installFontAwesome(s.flags, dist); err != nil {
			return fmt.Errorf("could not install fontawesome: %w", err)
		}
		// FIXME: other than for debugging purposes, is it necessary to write
		// FIXME: the manifest to disk?
		// write temporary manifest
		manifest, err := dist.ManifestBytes()
		if err != nil {
			return fmt.Errorf("could not generate manifest: %w", err)
		}
		if err := ioutil.WriteFile(filepath.Join(s.flags.Build, "manifest.json"), manifest, 0644); err != nil {
			return fmt.Errorf("could not write manifest.json: %w", err)
		}
		return filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
			switch {
			case err != nil:
				return err
			case fi.IsDir() || filepath.Dir(n) != dir || !strings.HasSuffix(n, "scss"):
				return nil
			}
			base := filepath.Base(n)
			if strings.HasPrefix(base, "_") || strings.HasPrefix(base, ".") {
				return nil
			}
			// build node-sass params
			fn := strings.TrimSuffix(base, ".scss")
			params := []string{
				"--quiet",
				"--source-comments",
				"--source-map-embed",
				//"--source-map-contents",
				//"--source-map=" + filepath.Join(s.flags.Build, cssDir,  fn + ".css.map"),
				//"--source-map-root=" + s.flags.Wd,
				"--functions=" + filepath.Join(s.flags.Build, sassJs),
				"--output=" + filepath.Join(s.flags.Build, cssDir),
				"--include-path=" + filepath.Join(s.flags.Build, "assetgen"),
				"--include-path=" + filepath.Join(s.flags.Build, "fontawesome"),
			}
			for _, z := range s.sassIncludes {
				params = append(params, "--include-path="+z)
			}
			// run node-sass
			if err := run(s.flags, "node-sass", append(params, n)...); err != nil {
				return fmt.Errorf("could not run node-sass: %w", err)
			}
			tailwindCss := filepath.Join(s.flags.Build, cssDir, fn+".tailwind.css")
			cleanCss := filepath.Join(s.flags.Build, cssDir, fn+".cleancss.css")
			finalCss := filepath.Join(s.flags.Build, cssDir, fn+".final.css")
			// tailwind
			if err := run(
				s.flags,
				"tailwindcss-cli",
				"build",
				filepath.Join(s.flags.Build, cssDir, fn+".css"),
				"-o", tailwindCss,
			); err != nil {
				return fmt.Errorf("could not run tailwind: %w", err)
			}
			// cleancss
			if err := runSilent(
				s.flags,
				"cleancss",
				"-O1", "specialComments:0",
				"-O2",
				"--inline", "all",
				"--source-map",
				"--output="+cleanCss,
				tailwindCss,
			); err != nil {
				return fmt.Errorf("could not run cleancss: %w", err)
			}
			// strip annoying comments
			buf, err := ioutil.ReadFile(cleanCss)
			if err != nil {
				return fmt.Errorf("could not read cleancss: %w", err)
			}
			// write final css
			buf = stripCssCommentsRE.ReplaceAll(buf, nil)
			if err := ioutil.WriteFile(finalCss, buf, 0644); err != nil {
				return fmt.Errorf("could not write final css: %w", err)
			}
			return dist.PackFile(cssDir+"/"+fn+".css", finalCss)
		})
	})
}

// addTemplates configures a script step for generating optimized template
// output (ie, Go code) from quicktemplate'd HTML files.
//
// This looks at the templates directory, and if there are any .html files,
// minifies them and normalizes templated i18n translation calls (T) before
// passing the template through the quicktemplate compiler (qtc).
func (s *Script) addTemplates(_, dir string) {
	// add htmlmin dependency
	s.nodeDeps = append(s.nodeDeps, dep{"html-minifier", ""})
	s.exec = append(s.exec, func(dist *pack.Pack) error {
		wd, err := os.Getwd()
		if err != nil {
			return err
		}
		tMatchRE, tFixRE, space := regexp.MustCompile(s.flags.TFuncName+"\\(`[^`]+`"), regexp.MustCompile(`\s+`), []byte(" ")
		err = filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
			switch {
			case err != nil:
				return err
			case fi.IsDir() || !strings.HasSuffix(n, ".html"):
				return nil
			}
			// read and minimize
			buf, err := ioutil.ReadFile(n)
			if err != nil {
				return err
			}
			min, err := htmlmin(s.flags, buf)
			if err != nil {
				return err
			}
			// change to the directory (necessary for qtc's parser to work)
			d := filepath.Dir(n)
			if err := os.Chdir(d); err != nil {
				return err
			}
			// generate go template
			out := new(bytes.Buffer)
			if err := qtcparser.Parse(out, bytes.NewReader(min), filepath.Base(n), filepath.Base(d)); err != nil {
				return err
			}
			// fix T(``) strings
			buf = tMatchRE.ReplaceAllFunc(out.Bytes(), func(b []byte) []byte {
				return tFixRE.ReplaceAll(b, space)
			})
			return ioutil.WriteFile(n+".go", buf, 0644)
		})
		if err != nil {
			defer func() {
				if err := os.Chdir(wd); err != nil {
					panic(err)
				}
			}()
			return err
		}
		return os.Chdir(wd)
	})
}

// ConfigDeps handles configuring dependencies.
func (s *Script) ConfigDeps() error {
	// load package.json
	buf, err := ioutil.ReadFile(filepath.Join(s.flags.Wd, "package.json"))
	if err != nil {
		return err
	}
	var v struct {
		Deps map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(buf, &v); err != nil {
		return errors.New("invalid package.json")
	}
	// build params
	params := []string{"add", "--no-progress", "--silent", "--no-bin-links", "--modules-folder=" + s.flags.NodeModules}
	var add bool
	for _, d := range s.nodeDeps {
		if _, ok := v.Deps[d.name]; ok {
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
	return run(s.flags, s.flags.YarnBin, params...)
}

// Execute executes the script.
func (s *Script) Execute(dist *pack.Pack) error {
	for _, f := range s.exec {
		if err := f(dist); err != nil {
			return err
		}
	}
	return nil
}

// startCallbackServer creates and starts the IPC callback server.
func (s *Script) startCallbackServer(ctxt context.Context, dist *pack.Pack) (string, error) {
	cbs, err := NewIpcServer(map[string]func(...interface{}) (interface{}, error){
		// asset($url) converts the passed url to a static path.
		"asset($url)": func(v ...interface{}) (interface{}, error) {
			// check args
			if len(v) != 1 {
				return nil, errors.New("invalid number of args")
			}
			z, ok := v[0].(string)
			if !ok {
				return nil, errors.New("$url must be a string")
			}
			// fix webfonts path (fontawesome)
			if strings.HasPrefix(z, "../webfonts/") {
				z = z[2:]
			}
			// save query string
			var qstr string
			if i := strings.LastIndex(z, "?"); i != -1 {
				qstr, z = z[i:], z[:i]
			} else if i := strings.LastIndex(z, "#"); i != -1 {
				qstr, z = z[i:], z[:i]
			}
			// grab manifest
			m, err := dist.Manifest()
			if err != nil {
				return nil, fmt.Errorf("unable to load manifest: %w", err)
			}
			// find asset name
			n, ok := m["/"+strings.TrimPrefix(z, "/")]
			if !ok {
				warnf(s.flags, "no asset %q in manifest", z)
				n = fmt.Sprintf("__INV:%s%s__", z, qstr)
			}
			return fmt.Sprintf("url('/_/%s%s')", n, qstr), nil
		},
		// googlefont($font) downloads the google font.
		"googlefont($font)": func(v ...interface{}) (interface{}, error) {
			fonts := []map[string]string{
				map[string]string{
					"font-family": "''",
				},
			}
			return fonts, nil
		},
	})
	if err != nil {
		return "", err
	}
	if err := cbs.Run(ctxt); err != nil {
		return "", err
	}
	return cbs.SocketPath(), nil
}

// findNodeModulesFile searches node_modules package for a masked file path,
// returning the path.
//
// If the passed dependency does not include a set file path, then it is
// assumed to be "<package name>.js". Searches first in the package's root,
// then the sub-directories dist and src. The first file matching the masked
// path name will be returned.
func (s *Script) findNodeModulesFile(jd jsdep) (string, error) {
	var found string
	if jd.path == "" {
		jd.path = jd.name + ".js"
	}
	dir := filepath.Join(s.flags.NodeModules, jd.name)
	err := filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case fi.IsDir() || found != "":
			return nil
		}
		for _, d := range []string{"", "dist", "src"} {
			pat, err := glob.Compile(filepath.Join(dir, d, jd.path))
			if err != nil {
				return fmt.Errorf("invalid path %q: %w", jd.path, err)
			}
			if pat.Match(n) {
				found = n
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if found == "" {
		return "", fmt.Errorf("could not find %q in npm package %s", jd.path, jd.name)
	}
	return found, nil
}

// fixNodeModulesBinLinks walks all packages in flags.NodeModules, reading their bin entries from
// package.json, and creating the appropriate symlink in flags.NodeModulesBin.
func fixNodeModulesBinLinks(flags *Flags) error {
	// ensure directory exists
	if err := checkDirs(flags, &flags.NodeModulesBin); err != nil {
		return fmt.Errorf("unable to fix node_modules/.bin: %w", err)
	}
	// erase all links in bin dir
	err := filepath.Walk(flags.NodeModulesBin, func(path string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case path == flags.NodeModulesBin:
			return nil
		case fi.Mode()&os.ModeSymlink == 0:
			return fmt.Errorf("%s is not a symlink", path)
		}
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("unable to remove %s: %w", path, err)
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
	err = filepath.Walk(flags.NodeModules, func(path string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case fi.IsDir() || filepath.Base(path) != "package.json":
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
		if err := json.Unmarshal(buf, &pkgDesc); err != nil {
			warnf(flags, "could not unmarshal %s: %w", path, err)
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
			rel, err := filepath.Rel(flags.NodeModules, z.dir)
			if err != nil {
				return fmt.Errorf("could not determine node-relative path for %s: %w", z.dir, err)
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
			return fmt.Errorf("unable to determine path for %s: %w", linkpath, err)
		}
		newname := filepath.Join(flags.NodeModulesBin, n)
		// check symlink exists
		_, err = os.Stat(newname)
		switch {
		case os.IsNotExist(err):
		case err != nil:
			return err
		}
		// symlink
		if err := os.Symlink(oldname, newname); err != nil {
			return fmt.Errorf("unable to symlink %s to %s: %w", newname, oldname, err)
		}
		// fix permissions
		if runtime.GOOS != "windows" {
			if err := os.Chmod(linkpath, 0755); err != nil {
				return err
			}
		}
	}
	return nil
}
