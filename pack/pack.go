package pack

import (
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/afero"
	"github.com/yookoala/realpath"
)

// Pack packs file assets.
type Pack struct {
	fs       afero.Fs
	h        map[string]string
	manifest string
	sync.RWMutex
}

// New creates a new asset packer.
func New(fs afero.Fs, opts ...Option) *Pack {
	p := &Pack{
		fs:       fs,
		h:        make(map[string]string),
		manifest: "manifest.json",
	}
	for _, o := range opts {
		o(p)
	}
	return p
}

// NewBase creates a new asset packer for the base path.
func NewBase(base string, opts ...Option) (*Pack, error) {
	base, err := realpath.Realpath(base)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		return nil, err
	}
	return New(afero.NewBasePathFs(afero.NewOsFs(), base), opts...), nil
}

// Pack packs a file with name copying the contents from r.
func (p *Pack) Pack(name string, r io.Reader) error {
	p.Lock()
	defer p.Unlock()
	name = "/" + strings.TrimLeft(name, "/")
	buf, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	if err := p.fs.MkdirAll(filepath.Dir(name), 0755); err != nil {
		return err
	}
	if err := afero.WriteFile(p.fs, name, buf, 0644); err != nil {
		return err
	}
	p.h[name] = fmt.Sprintf("%x", md5.Sum(buf))
	return nil
}

// PackBytes packs a file with name with contents of buf.
func (p *Pack) PackBytes(name string, buf []byte) error {
	return p.Pack(name, bytes.NewReader(buf))
}

// PackString packs a file with name with contents of s.
func (p *Pack) PackString(name string, s string) error {
	return p.Pack(name, strings.NewReader(s))
}

// PackFile packs a file with name with the contents read from the specified
// path.
func (p *Pack) PackFile(name, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return p.Pack(name, f)
}

// Manifest returns a manifest of the packed files.
func (p *Pack) Manifest() (map[string]string, error) {
	p.RLock()
	defer p.RUnlock()
	m := make(map[string]string)
	err := afero.Walk(p.fs, "/", func(n string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case fi.IsDir() || filepath.Base(n) == p.manifest:
			return nil
		}
		fh := fmt.Sprintf("%x", md5.Sum([]byte(strings.TrimLeft(n, "/"))))
		m[n] = fh[:6] + "." + p.h[n][:6] + filepath.Ext(n)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return m, nil
}

// ManifestInverted returns a manifest of the packed files (inverted).
func (p *Pack) ManifestInverted() (map[string]string, error) {
	m, err := p.Manifest()
	if err != nil {
		return nil, err
	}
	rev := make(map[string]string, len(m))
	for v, k := range m {
		rev[k] = v
	}
	return rev, nil
}

// ManifestBytes returns a JSON-encoded version of the file manifest.
func (p *Pack) ManifestBytes() ([]byte, error) {
	m, err := p.Manifest()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// ManifestInvertedBytes returns a JSON-encoded version of the file manifest
// (inverted).
func (p *Pack) ManifestInvertedBytes() ([]byte, error) {
	m, err := p.ManifestInverted()
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(m, "", "  ")
}

// WriteManifest writes the file manifest.
func (p *Pack) WriteManifest() error {
	buf, err := p.ManifestBytes()
	if err != nil {
		return err
	}
	return afero.WriteFile(p.fs, p.manifest, buf, 0644)
}

// WriteManifestInverted writes the file manifest (inverted).
func (p *Pack) WriteManifestInverted() error {
	buf, err := p.ManifestInvertedBytes()
	if err != nil {
		return err
	}
	return afero.WriteFile(p.fs, p.manifest, buf, 0644)
}

// Option is an asset packer option.
type Option func(*Pack)

// WithManifest is an asset packer option to set the manifest name.
func WithManifest(manifest string) Option {
	return func(p *Pack) {
		p.manifest = manifest
	}
}
