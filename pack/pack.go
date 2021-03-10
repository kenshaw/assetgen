package pack

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/shurcooL/vfsgen"
	"github.com/spf13/afero"
)

// Pack handles packing binary assets into a Go package.
type Pack struct {
	fs  afero.Fs
	h   map[string]string
	pkg string
	sync.RWMutex
}

// New creates a new binary asset packer with the specified pkg name.
func New(opts ...Option) *Pack {
	p := &Pack{
		fs: afero.NewMemMapFs(),
		h:  make(map[string]string),
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// Add adds a file with name to pack from r.
func (p *Pack) Add(name string, r io.Reader) error {
	p.Lock()
	defer p.Unlock()
	name = "/" + strings.TrimLeft(name, "/")
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	err = p.fs.MkdirAll(filepath.Dir(name), 0755)
	if err != nil {
		return err
	}
	f, err := p.fs.Create(name)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(buf)
	if err != nil {
		return err
	}
	h := md5.Sum(buf)
	p.h[name] = hex.EncodeToString(h[:])
	return nil
}

// AddFile adds a file with name to the output with the contents of the file at path.
func (p *Pack) AddFile(name string, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return p.Add(name, f)
}

// AddBytes adds a file with name to the output from buf.
func (p *Pack) AddBytes(name string, buf []byte) error {
	return p.Add(name, bytes.NewReader(buf))
}

// AddString adds a file with name to the output from s.
func (p *Pack) AddString(name string, s string) error {
	return p.Add(name, strings.NewReader(s))
}

// Manifest returns a manifest of the packed file data.
func (p *Pack) Manifest() (map[string]string, error) {
	p.RLock()
	defer p.RUnlock()
	m := make(map[string]string)
	err := afero.Walk(p.fs, "/", func(n string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case fi.IsDir():
			return nil
		}
		h := md5.Sum([]byte(strings.TrimLeft(n, "/")))
		fh := hex.EncodeToString(h[:])
		m[n] = fh[:6] + "." + p.h[n][:6] + filepath.Ext(n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ManifestBytes returns a JSON-encoded version of the file manifest.
func (p *Pack) ManifestBytes() ([]byte, error) {
	m, err := p.Manifest()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// WriteTo writes to the specified out file, with the specified variable name.
func (p *Pack) WriteTo(out, name string) error {
	p.RLock()
	defer p.RUnlock()
	pkg := p.pkg
	if pkg == "" {
		pkg = filepath.Base(filepath.Dir(out))
	}
	return vfsgen.Generate(p, vfsgen.Options{
		VariableName:  name,
		Filename:      out,
		PackageName:   pkg,
		ForceAllTypes: true,
	})
}

// Open satisfies the http.FileSystem interface.
func (p *Pack) Open(name string) (http.File, error) {
	p.RLock()
	defer p.RUnlock()
	return p.fs.Open(name)
}
