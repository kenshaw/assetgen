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
	"strings"
	"time"

	"github.com/mattn/anko/vm"
	qtcparser "github.com/valyala/quicktemplate/parser"
	"golang.org/x/sync/errgroup"
	"golang.org/x/tools/imports"

	"github.com/brankas/assetgen/pack"
)

// dep wraps package dependency information.
type dep struct {
	name string
	ver  string
}

// Script wraps an assetgen script.
type Script struct {
	flags *Flags

	// logf is the log func.
	logf func(string, ...interface{})

	// deps are package dependencies.
	nodeDeps []dep

	// sassIncludes are sass include directories.
	sassIncludes []string

	// pre are the pre setup steps to be executed in order.
	pre []func() error

	// exec is the steps to be executed, in order.
	exec []func() error

	// post are the post setup steps to be executed in order.
	post []func() error

	// dist is the assets to distribute (ie, pack).
	dist *pack.Pack
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
		logf:  log.Printf,
		dist:  pack.New("assets"),
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
		{"sass", s.addSass},
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

// get retrieves src.
func (s *Script) get(src string) ([]byte, error) {
	res, err := http.Get(src)
	if err != nil {
		return nil, fmt.Errorf("could not retrieve %q: %v", src, err)
	}
	defer res.Body.Close()
	return ioutil.ReadAll(res.Body)
}

// getAndPack retrieves src and writes it to the dest file.
func (s *Script) getAndPack(dest, src, name string) error {
	s.logf("GET %s => %s", src, dest)
	buf, err := s.get(src)
	if err != nil {
		return err
	}

	// write packed data
	p := pack.New(filepath.Base(filepath.Dir(dest)))
	p.AddBytes(filepath.Base(src), buf)
	return p.WriteTo(dest, name)
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

// addFonts configures a script step for packing static font files.
//
// This walks the fonts directory, and if there's a SCSS/CSS file, add it to
// sass import path. All font files will be added to the manifest.
func (s *Script) addFonts(_, dir string) {
}

// addGeoip configures a script step for packing geoip data.
//
// This looks at the geoip directory, and if there is no geoip data, downloads
// the appropriate file (if it doesn't exist), and adds the file to the packed
// data (but not to the manifest).
func (s *Script) addGeoip(_, dir string) {
	s.exec = append(s.exec, func() error {
		path := filepath.Join(dir, "geoip.go")
		fi, err := os.Stat(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("could not stat %s: %v", path, err)
		}

		// bail if data isn't stale
		if err == nil && s.flags.Ttl != 0 && !time.Now().After(fi.ModTime().Add(s.flags.Ttl)) {
			return nil
		}

		// download and cache
		return s.getAndPack(path, geoipURL, "Geoip")
	})
}

var imageExtRE = regexp.MustCompile(`(?i)\.(jpe?g|gif|png|svg|mp4|json)$`)

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

	s.exec = append(s.exec, func() error {
		cacheDir := s.flags.Cache + "/images/"

		// ensure image caching dir exists
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return err
		}

		// accumulate images
		var all, changed []string
		err = filepath.Walk(dir, func(n string, f os.FileInfo, err error) error {
			// skip non images
			if !imageExtRE.MatchString(f.Name()) {
				return nil
			}

			// hash
			fn := strings.TrimPrefix(n, dir+"/")
			hash, err := md5hash(n)
			if err != nil {
				return err
			}
			hashPath := cacheDir + fn + ".md5"

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
			if cached == "" || cached != hash {
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
						if err := s.optimizeImage(cacheDir+fn, dir+"/"+fn); err != nil {
							return err
						}
					}
				}
			})
		}
		if err = eg.Wait(); err != nil {
			return err
		}

		// pack the generated images
		for _, fn := range all {
			if err := s.dist.AddFile("images/"+fn, cacheDir+fn); err != nil {
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

// addSass configures a script step for compiling and minifying sass assets.
//
// This walks the sass directory, and if there's any .scss files, generates the
// appropriate css after compiling, prefixing, and minifying.
func (s *Script) addSass(_, dir string) {
	for _, n := range []string{
		"node-sass",
		"postcss-cli",
		"autoprefixer",
		"clean-css-cli",
	} {
		s.nodeDeps = append(s.nodeDeps, dep{n, ""})
	}

	s.exec = append(s.exec, func() error {
		// write temporary manifest
		manifest, err := s.dist.ManifestBytes()
		if err != nil {
			return fmt.Errorf("could not generate manifest: %v", err)
		}
		if err = ioutil.WriteFile(s.flags.Build+"/manifest.json", manifest, 0644); err != nil {
			return fmt.Errorf("could not write manifest.json: %v", err)
		}

		// write sass.js to build dir
		err = ioutil.WriteFile(s.flags.Build+"/sass.js", []byte(sassJsTemplate+"\n"), 0644)
		if err != nil {
			return fmt.Errorf("could not write sass.js: %v", err)
		}

		return filepath.Walk(dir, func(n string, fi os.FileInfo, err error) error {
			switch {
			case err != nil:
				return err
			case fi.IsDir() || filepath.Dir(n) != dir || strings.HasPrefix(n, "_") || !strings.HasSuffix(n, "scss"):
				return nil
			}

			// build node-sass params
			fn := strings.TrimSuffix(filepath.Base(n), ".scss")
			params := []string{
				"--quiet",
				"--source-comments",
				"--source-map-contents",
				"--source-map=" + s.flags.Build + "/css/" + fn + ".css.map",
				"--functions=" + s.flags.Build + "/sass.js",
				"--output=" + s.flags.Build + "/css",
			}
			for _, z := range s.sassIncludes {
				params = append(params, "--include-path="+z)
			}

			// run node-sass
			err = runSilent(s.flags, "node-sass", append(params, n)...)
			if err != nil {
				return fmt.Errorf("could not run node-sass: %v", err)
			}

			// autoprefixer
			err = runSilent(
				s.flags,
				"postcss",
				"--use=autoprefixer",
				"--map",
				"--output="+s.flags.Build+"/css/"+fn+".postcss.css",
				s.flags.Build+"/css/"+fn+".css",
			)
			if err != nil {
				return fmt.Errorf("could not run postcss: %v", err)
			}

			// cleancss
			err = runSilent(
				s.flags,
				"cleancss",
				"-O2",
				"--format='specialComments:0;processImport:0'",
				"--source-map",
				"--skip-rebase",
				"--output="+s.flags.Build+"/css/"+fn+".cleancss.css",
				s.flags.Build+"/css/"+fn+".postcss.css",
			)
			if err != nil {
				return fmt.Errorf("could not run cleancss: %v", err)
			}

			return s.dist.AddFile("css/"+fn+".css", s.flags.Build+"/css/"+fn+".cleancss.css")
		})
	})
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
func (s *Script) addTemplates(_, dir string) {
	// add htmlmin dependency
	s.nodeDeps = append(s.nodeDeps, dep{"html-minifier", ""})

	s.exec = append(s.exec, func() error {
		var err error

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
			if err = os.Chdir(d); err != nil {
				return err
			}

			// generate go template
			out := new(bytes.Buffer)
			if err = qtcparser.Parse(out, bytes.NewReader(min), filepath.Base(n), filepath.Base(d)); err != nil {
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
	err = json.Unmarshal(buf, &v)
	if err != nil {
		return errors.New("invalid package.json")
	}

	// build params
	params := []string{"add", "--no-progress", "--silent"}
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

	return run(s.flags, s.flags.Yarn, params...)
}

// Execute executes the script.
func (s *Script) Execute() error {
	var err error
	for _, f := range s.exec {
		if err = f(); err != nil {
			return err
		}
	}

	manifest, err := s.dist.Manifest()
	if err != nil {
		return err
	}
	rev := make(map[string]string, len(manifest))
	for k, v := range manifest {
		rev[v] = k
	}

	if err = s.dist.AddBytes("ok", []byte(`ok`)); err != nil {
		return err
	}

	if err = s.dist.WriteTo(s.flags.Assets+"/assets.go", "Assets"); err != nil {
		return err
	}

	fn := s.flags.Assets + "/manifest.go"
	buf, err := imports.Process(fn, []byte(fmt.Sprintf(manifestTemplate, manifest, rev)), nil)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(fn, buf, 0644)
}

const (
	manifestTemplate = `package assets

import (
	"fmt"
	"net/http"
	"os"
	"time"
	"strings"

	"github.com/shurcooL/httpfs/vfsutil"
	"github.com/shurcooL/httpgzip"
)

// ManifestAssets returns the the manifest assets as http.FileSystem.
var ManifestAssets = func() http.FileSystem {
	fs := vfsgen۰FS{
		"/": &vfsgen۰DirInfo{
			name:    "/",
			modTime: time.Time{},
		},
	}

	manifest := Manifest()
	err := vfsutil.Walk(Assets, "/", func(n string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case fi.IsDir() || n == "/ok":
			return nil
		}

		f, ok := Assets.(vfsgen۰FS)[n]
		if !ok {
			return fmt.Errorf("no asset %%s", n)
		}

		fn, ok := manifest[n]
		if !ok {
			return fmt.Errorf("could not find path for %%s", n)
		}
		fn = "/" + fn

		var z interface{}
		switch x := f.(type) {
		case *vfsgen۰CompressedFileInfo:
			z = &vfsgen۰CompressedFileInfo{
				name: fn,
				modTime: x.modTime,
				compressedContent: x.compressedContent,
				uncompressedSize: x.uncompressedSize,
			}
		case *vfsgen۰FileInfo:
			z = &vfsgen۰FileInfo{
				name: x.name,
				modTime: x.modTime,
				content: x.content,
			}
		}
		fs[fn] = z
		fs["/"].(*vfsgen۰DirInfo).entries = append(fs["/"].(*vfsgen۰DirInfo).entries, z.(os.FileInfo))
		return nil
	})
	if err != nil {
		panic(err)
	}

	return fs
}()

type vfsgen۰Handler struct {
	h http.Handler
}

func (h *vfsgen۰Handler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	z, u := *req, *req.URL
	z.URL = &u
	z.URL.Path = strings.TrimPrefix(z.URL.Path, "/_")
	h.h.ServeHTTP(res, &z)
}

// StaticHandler returns a static asset handler, with f handling any errors
// encountered.
func StaticHandler(f func(http.ResponseWriter, *http.Request, error)) http.Handler {
	return &vfsgen۰Handler{
		h: httpgzip.FileServer(ManifestAssets, httpgzip.FileServerOptions{
			ServeError: f,
		}),
	}
}

// Manifest returns the asset manifest.
func Manifest() map[string]string {
	return %#v
}

// ReverseManifest returns the reverse asset manifest.
func ReverseManifest() map[string]string {
	return %#v
}`
)
