package gen

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/Masterminds/semver"
	"github.com/shurcooL/httpfs/vfsutil"
	"github.com/shurcooL/httpgzip"
)

// warnf handles issuing warnings.
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
