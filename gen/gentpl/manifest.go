package assets

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha1"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	"github.com/shurcooL/httpfs/vfsutil"
)

// vfsgen۰Asset is a static asset.
type vfsgen۰Asset struct {
	Data        []byte
	ContentType string
	ModTime     time.Time
	SHA1        string
}

// vfsgen۰buildManifestAssets builds manifest assets.
func vfsgen۰buildManifestAssets() (map[string]vfsgen۰Asset, error) {
	manifest := Manifest()
	assets := make(map[string]vfsgen۰Asset, len(manifest))
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

		var data []byte
		switch x := f.(type) {
		case *vfsgen۰CompressedFileInfo:
			r, err := gzip.NewReader(bytes.NewReader(x.compressedContent))
			if err != nil {
				return err
			}
			data, err = ioutil.ReadAll(r)
			if err != nil {
				return err
			}
		case *vfsgen۰FileInfo:
			data = x.content
		}

		assets[fn] = vfsgen۰Asset{
			Data:        data,
			ContentType: http.DetectContentType(data),
			ModTime:     fi.ModTime(),
			SHA1:        fmt.Sprintf("%%x", sha1.Sum(data)),
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return assets, nil
}

// StaticHandler returns the manifest path static asset handler.
func StaticHandler(urlpath func(context.Context) string) http.Handler {
	if urlpath == nil {
		panic("urlpath func cannot be nil")
	}

	assets, err := vfsgen۰buildManifestAssets()
	if err != nil {
		panic(err)
	}

	return http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		// grab info
		asset, ok := assets[urlpath(req.Context())]
		if !ok {
			http.Error(res, http.StatusText(http.StatusNotFound), http.StatusNotFound)
			return
		}

		// check if-modified-since header, bail if present
		if t, err := time.Parse(http.TimeFormat, req.Header.Get("If-Modified-Since")); err == nil && asset.ModTime.Unix() <= t.Unix() {
			res.WriteHeader(http.StatusNotModified) // 304
			return
		}

		// check If-None-Match header, bail if present and match sha1
		if req.Header.Get("If-None-Match") == asset.SHA1 {
			res.WriteHeader(http.StatusNotModified) // 304
			return
		}

		// set headers
		res.Header().Set("Content-Type", asset.ContentType)
		res.Header().Set("Date", time.Now().Format(http.TimeFormat))

		// cache headers
		res.Header().Set("Cache-Control", "public, no-transform, max-age=31536000")
		res.Header().Set("Expires", time.Now().AddDate(1, 0, 0).Format(http.TimeFormat))
		res.Header().Set("Last-Modified", asset.ModTime.Format(http.TimeFormat))
		res.Header().Set("ETag", asset.SHA1)

		// write data to response
		res.Write(asset.Data)
	})
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
