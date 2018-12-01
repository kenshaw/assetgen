package pack

import (
	"bytes"
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/shurcooL/vfsgen"
	"github.com/spf13/afero"
)

// Pack handles packing binary assets into a Go package.
type Pack struct {
	pkg string
	fs  afero.Fs
}

// New creates a new binary asset packer.
func New(pkg string) *Pack {
	return &Pack{
		pkg: pkg,
		fs:  afero.NewMemMapFs(),
	}
}

// Add adds a file with name to pack from r.
func (p *Pack) Add(name string, r io.Reader) error {
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
	return err
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
	return nil, nil
}

// ManifestBytes returns a JSON-encoded version of the file manifest.
func (p *Pack) ManifestBytes() ([]byte, error) {
	m, err := p.Manifest()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// Pack packs binary assets.
func (p *Pack) WriteTo(out, name string) error {
	return vfsgen.Generate(p, vfsgen.Options{
		VariableName: name,
		Filename:     out,
		PackageName:  p.pkg,
	})
}

// Open satisfies the http.FileSystem interface.
func (p *Pack) Open(name string) (http.File, error) {
	return p.fs.Open(name)
}
