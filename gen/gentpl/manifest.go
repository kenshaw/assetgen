package assets

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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
		case fi.IsDir():
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
				name:              fn,
				modTime:           x.modTime,
				compressedContent: x.compressedContent,
				uncompressedSize:  x.uncompressedSize,
			}
		case *vfsgen۰FileInfo:
			z = &vfsgen۰FileInfo{
				name:    x.name,
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
	return vfsgen۰manifest
}

// ReverseManifest returns the reverse asset manifest.
func ReverseManifest() map[string]string {
	return vfsgen۰rev
}

// ManifestPath returns the asset path for name.
func ManifestPath(name string) string {
	return vfsgen۰manifest[name]
}

// ReversePath returns the manifest path for name.
func ReversePath(name string) string {
	return vfsgen۰rev[name]
}
