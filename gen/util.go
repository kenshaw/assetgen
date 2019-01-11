package gen

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/Masterminds/semver"
	"github.com/shurcooL/httpfs/vfsutil"
	"github.com/shurcooL/httpgzip"
)

// infof handles logging information.
func infof(flags *Flags, s string, v ...interface{}) {
	if flags.Verbose {
		log.Printf(s, v...)
	}
}

// warnf handles logging warnings.
func warnf(flags *Flags, s string, v ...interface{}) {
	if flags.Verbose {
		log.Printf("WARNING: "+s, v...)
	}
}

// formatCommand formats the command output
func formatCommand(name string, params ...string) string {
	paramstr := " " + strings.Join(params, " ")
	if (len(paramstr) + len(name)) >= 40 {
		paramstr = ""
		for _, p := range params {
			paramstr += " \\\n  " + p
		}
	}
	return name + paramstr
}

// run runs command name with params.
func run(flags *Flags, name string, params ...string) error {
	if flags.Verbose {
		fmt.Fprintln(os.Stdout, formatCommand(name, params...))
	}
	cmd := exec.Command(name, params...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.Dir = flags.Wd
	return cmd.Run()
}

// runSilent runs command name with params silently (ie, stdout is discarded).
func runSilent(flags *Flags, name string, params ...string) error {
	if flags.Verbose {
		fmt.Fprintln(os.Stdout, formatCommand(name, params...))
	}
	cmd := exec.Command(name, params...)
	cmd.Dir = flags.Wd
	return cmd.Run()
}

// runCombined runs command name with params, returning the trimmed, combined
// output of stdout and stderr.
func runCombined(flags *Flags, name string, params ...string) (string, error) {
	if flags.Verbose {
		fmt.Fprintln(os.Stdout, formatCommand(name, params...))
	}
	cmd := exec.Command(name, params...)
	cmd.Dir = flags.Wd
	buf, err := cmd.CombinedOutput()
	return string(bytes.TrimSpace(buf)), err
}

// compareSemver compares a semantic version against a constraint.
func compareSemver(version, constraint string) bool {
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		panic(fmt.Sprintf("invalid constraint %q: %v", constraint, err))
	}
	return c.Check(semver.MustParse(version))
}

// concat concatentates files and writes to out.
func concat(files []string, out string) error {
	var buf bytes.Buffer

	// process files
	for i, file := range files {
		if i != 0 {
			buf.WriteString("\n")
		}

		// read file
		b, err := ioutil.ReadFile(file)
		if err != nil {
			return err
		}

		// append to buf
		_, err = buf.Write(b)
		if err != nil {
			return err
		}
	}

	return ioutil.WriteFile(out, buf.Bytes(), 0644)
}

// cp recursively copies files from directory a to b that match the passed regexp.
func cp(a, b string, re *regexp.Regexp) error {
	err := os.MkdirAll(b, 0755)
	if err != nil {
		return err
	}

	return filepath.Walk(a, func(path string, f os.FileInfo, err error) error {
		fn := strings.TrimPrefix(path, a)
		switch {
		case err != nil:
			return err
		case fn == "":
			return nil
		case f.IsDir():
			return os.MkdirAll(filepath.Join(b, fn), f.Mode())
		case re.MatchString(f.Name()):
			src, err := os.Open(path)
			if err != nil {
				return err
			}
			defer src.Close()

			dst, err := os.Create(filepath.Join(b, fn))
			if err != nil {
				return err
			}
			defer dst.Close()

			_, err = io.Copy(dst, src)
			return err
		}
		return nil
	})
}

// isParentDir determines if b is a child directory of a.
//
// Note: if a, b, or any parents of b do not exist, this will panic.
func isParentDir(a, b string) bool {
	afi, err := os.Lstat(a)
	if err != nil {
		panic(fmt.Sprintf("dir %q must exist", a))
	}

	for b != "" {
		bfi, err := os.Lstat(b)
		if err != nil {
			panic(fmt.Sprintf("dir %q does not exist", b))
		}
		if os.SameFile(afi, bfi) {
			return true
		}
		n := filepath.Dir(b)
		if b == n {
			break
		}
		b = n
	}

	return false
}

// forceMap forces v to a map.
func forceMap(v interface{}, names ...string) map[string]string {
	if z, ok := v.(map[string]interface{}); ok {
		m := make(map[string]string, len(z))
		for a, b := range z {
			m[a] = forceString(b)
		}
		return m
	}

	var name string
	for _, n := range names {
		if n != "" {
			name = n
			break
		}
	}
	return map[string]string{
		name: forceString(v),
	}
}

// forceString forces v into a string representation.
func forceString(v interface{}) string {
	switch z := v.(type) {
	case string:
		return z
	default:
		return fmt.Sprintf("%v", z)
	}
	return ""
}

// htmlmin passes the supplied byte slice to html-minifier's stdin, returning
// the output.
func htmlmin(flags *Flags, buf []byte) ([]byte, error) {
	cmd := exec.Command(
		"html-minifier",
		"--collapse-boolean-attributes",
		"--collapse-whitespace",
		"--remove-comments",
		"--remove-attribute-quotes",
		"--remove-script-type-attributes",
		"--remove-style-link-type-attributes",
		"--minify-css",
		"--minify-js",
		`--ignore-custom-fragments="\\{%[^%]+%\\}"`,
		"--trim-custom-fragments",
	)
	cmd.Stdin = bytes.NewReader(buf)
	cmd.Dir = flags.Wd
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	defer out.Close()

	if err = cmd.Start(); err != nil {
		return nil, err
	}

	buf, err = ioutil.ReadAll(out)
	if err != nil {
		return nil, err
	}

	if err = cmd.Wait(); err != nil {
		return nil, err
	}

	return buf, nil
}

// isValidIdentifier determines if s is a valid Go identifier.
func isValidIdentifier(s string) bool {
	if len(s) == 0 || !unicode.IsLetter([]rune(s[0:1])[0]) {
		return false
	}

	for _, ch := range s {
		if !isIdentifierChar(ch) {
			return false
		}
	}

	return true
}

// isIdentifierChar returns true if ch is a valid identifier character.
func isIdentifierChar(ch rune) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_' || ch >= 0x80 && unicode.IsLetter(ch) ||
		'0' <= ch && ch <= '9' || ch >= 0x80 && unicode.IsDigit(ch)
}

// md5hash returns the md5 hash of the contents of file in hex format.
func md5hash(file string) (string, error) {
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return "", err
	}
	sum := md5.Sum(buf)
	return hex.EncodeToString(sum[:]), nil
}

// templates are loaded file assets used by assetgen.
var templates map[string]string

func init() {
	// walk and add all template assets
	templates = make(map[string]string)
	err := vfsutil.Walk(files, "/", func(n string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case fi.IsDir():
			return nil
		}
		f, err := files.Open(n)
		if err != nil {
			return err
		}
		defer f.Close()

		var buf []byte
		switch x := f.(type) {
		case httpgzip.GzipByter:
			r, err := gzip.NewReader(bytes.NewReader(x.GzipBytes()))
			if err != nil {
				return err
			}
			buf, err = ioutil.ReadAll(r)
			if err != nil {
				return err
			}
		case httpgzip.NotWorthGzipCompressing:
			buf, err = ioutil.ReadAll(f)
			if err != nil {
				return err
			}
		}

		templates[strings.TrimPrefix(n, "/")] = string(buf)
		return nil
	})
	if err != nil {
		panic(err)
	}
}

// tplf loads the named template, and fmt.Sprintf's v.
func tplf(name string, v ...interface{}) string {
	tpl, ok := templates[name]
	if !ok {
		panic(fmt.Sprintf("could not load template: %s", name))
	}
	return fmt.Sprintf(tpl, v...)
}

// fileExists returns true if name exists on disk.
func fileExists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	}
	return !os.IsNotExist(err)
}

// getAndCache retrieves the specified file, caching it to the specified path.
func getAndCache(flags *Flags, urlstr string, ttl time.Duration, b64decode bool, names ...string) ([]byte, error) {
	n := pathJoin(flags.Cache, names...)
	cd := filepath.Dir(n)
	err := os.MkdirAll(cd, 0755)
	if err != nil {
		return nil, err
	}

	// check if file exists on disk
	fi, err := os.Stat(n)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return nil, err
	case ttl == 0 || !time.Now().After(fi.ModTime().Add(ttl)):
		return ioutil.ReadFile(n)
	}

	infof(flags, "RETRIEVING: %s", urlstr)

	// retrieve
	cl := &http.Client{}
	req, err := http.NewRequest("GET", urlstr, nil)
	if err != nil {
		return nil, err
	}
	res, err := cl.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("could not retrieve %q (%d)", urlstr, res.StatusCode)
	}

	buf, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	// decode
	if b64decode {
		buf, err = base64.StdEncoding.DecodeString(string(buf))
		if err != nil {
			return nil, err
		}
	}

	// write
	if err = ioutil.WriteFile(n, buf, 0644); err != nil {
		return nil, err
	}

	return buf, nil
}

// pathJoin is a simple wrapper around filepath.Join to simplify inline syntax.
func pathJoin(n string, m ...string) string {
	return filepath.Join(append([]string{n}, m...)...)
}

// extractArchive extracts buf to dir.
func extractArchive(dir string, buf []byte, ext string, chop string) error {
	switch ext {
	case ".zip":
		return extractZip(dir, buf, chop)
	case ".tar.gz":
		return extractTarGz(dir, buf, chop)
	}
	return fmt.Errorf("invalid archive type %q", ext)
}

// extractZip extracts buf to dir.
func extractZip(dir string, buf []byte, chop string) error {
	r, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return err
	}

	for _, z := range r.File {
		n := filepath.Join(dir, strings.TrimPrefix(z.Name, chop))
		fi := z.FileInfo()
		switch {
		case fi.IsDir():
			if err = os.MkdirAll(n, fi.Mode()); err != nil {
				return err
			}

		default:
			fr, err := z.Open()
			if err != nil {
				return err
			}
			f, err := os.OpenFile(n, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, fi.Mode())
			if err != nil {
				return err
			}
			if _, err = io.Copy(f, fr); err != nil {
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}
			if err = fr.Close(); err != nil {
				return err
			}
		}
	}

	return nil
}

// extractTarGz extracts buf to dir.
func extractTarGz(dir string, buf []byte, chop string) error {
	gz, err := gzip.NewReader(bytes.NewReader(buf))
	if err != nil {
		return err
	}

	r := tar.NewReader(gz)

loop:
	for {
		// next file
		h, err := r.Next()
		switch {
		case err == io.EOF:
			break loop
		case err != nil:
			return err
		}

		n := filepath.Join(dir, strings.TrimPrefix(h.Name, chop))
		switch h.Typeflag {
		case tar.TypeDir:
			// create dir
			if err = os.MkdirAll(n, h.FileInfo().Mode()); err != nil {
				return err
			}

		case tar.TypeReg:
			// write file
			f, err := os.OpenFile(n, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, h.FileInfo().Mode())
			if err != nil {
				return err
			}
			if _, err = io.Copy(f, r); err != nil {
				return err
			}
			if err = f.Close(); err != nil {
				return err
			}

		case tar.TypeSymlink:
			// check that symlink is contained in dir and link
			p := filepath.Clean(filepath.Join(filepath.Dir(n), h.Linkname))
			if _, err = filepath.Rel(dir, p); err != nil {
				return fmt.Errorf("could not make tar symlink %q relative to %s", h.Linkname, dir)
			}
			if err = os.Symlink(p, n); err != nil {
				return fmt.Errorf("could not create symlink for %q: %v", n, err)
			}

		default:
			return fmt.Errorf("unsupported file type in tar: %v", h.Typeflag)
		}
	}
	return nil
}

type githubAsset struct {
	BrowserDownloadURL string `json:"browser_download_url"`
	Name               string `json:"name"`
	ContentType        string `json:"content_type"`
}

// githubLatestAssets retrieves the latest release assets from the named repo.
func githubLatestAssets(flags *Flags, repo, dir string) (string, []githubAsset, error) {
	urlstr := "https://api.github.com/repos/" + repo + "/releases/latest"
	buf, err := getAndCache(flags, urlstr, flags.Ttl, false, dir, "latest.json")
	if err != nil {
		return "", nil, err
	}

	var release struct {
		Name   string        `json:"name"`
		Assets []githubAsset `json:"assets"`
	}
	if err = json.Unmarshal(buf, &release); err != nil {
		return "", nil, err
	}

	return release.Name, release.Assets, nil
}
