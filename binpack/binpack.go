// Package binpack converts any file into managable Go source code. Useful for
// embedding binary data into a Go program. The file data is optionally gzip
// compressed before being converted to a raw byte slice.
//
// The following paragraphs cover some of the customization options which can
// be specified in the binpack struct, which must be passed into the Translate()
// call.
//
// Debug vs Release builds
//
// When used with the `Debug` option, the generated code does not actually
// include the asset data. Instead, it generates function stubs which load the
// data from the original file on disk. The asset API remains identical between
// debug and release builds, so your code will not have to change.
//
// This is useful during development when you expect the assets to change
// often.  The host application using these assets uses the same API in both
// cases and will not have to care where the actual data comes from.
//
// An example is a Go webserver with some embedded, static web content like
// HTML, JS and CSS files. While developing it, you do not want to rebuild the
// whole server and restart it every time you make a change to a bit of
// javascript. You just want to build and launch the server once. Then just
// press refresh in the browser to see those changes. Embedding the assets with
// the `debug` flag allows you to do just that. When you are finished
// developing and ready for deployment, just re-invoke `binpack` without the
// `-debug` flag.  It will now embed the latest version of the assets.
//
// Lower memory footprint
//
// The `NoMemCopy` option will alter the way the output file is generated.  It
// will employ a hack that allows us to read the file data directly from the
// compiled program's `.rodata` section. This ensures that when we call call
// our generated function, we omit unnecessary memcopies.
//
// The downside of this, is that it requires dependencies on the `reflect` and
// `unsafe` packages. These may be restricted on platforms like AppEngine and
// thus prevent you from using this mode.
//
// Another disadvantage is that the byte slice we create, is strictly
// read-only.  For most use-cases this is not a problem, but if you ever try to
// alter the returned byte slice, a runtime panic is thrown. Use this mode only
// on target platforms where memory constraints are an issue.
//
// The default behaviour is to use the old code generation method. This
// prevents the two previously mentioned issues, but will employ at least one
// extra memcopy and thus increase memory requirements.
//
// For instance, consider the following two examples:
//
// This would be the default mode, using an extra memcopy but gives a safe
// implementation without dependencies on `reflect` and `unsafe`:
//
// 	func myfile() []byte {
//      return []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a}
//  }
//
// Here is the same functionality, but uses the `.rodata` hack.  The byte slice
// returned from this example can not be written to without generating a
// runtime error.
//
// 	var _myfile = "\x89\x50\x4e\x47\x0d\x0a\x1a"
//
// 	func myfile() []byte {
//     var empty [0]byte
//     sx := (*reflect.StringHeader)(unsafe.Pointer(&_myfile))
//     b := empty[:]
//     bx := (*reflect.SliceHeader)(unsafe.Pointer(&b))
//     bx.Data = sx.Data
//     bx.Len = len(_myfile)
//     bx.Cap = bx.Len
//     return b
//  }
//
// Optional compression
//
// The NoCompress option indicates that the supplied assets are *not* GZIP
// compressed before being turned into Go code. The data should still be
// accessed through a function call, so nothing changes in the API.
//
// This feature is useful if you do not care for compression, or the supplied
// resource is already compressed. Doing it again would not add any value and
// may even increase the size of the data.
//
// The default behaviour of the program is to use compression.
//
// Path prefix stripping
//
// The keys used in the `_bindata` map are the same as the input file name
// passed to `binpack`. This includes the path. In most cases, this is not
// desireable, as it puts potentially sensitive information in your code base.
// For this purpose, the tool supplies another command line flag `-prefix`.
// This accepts a portion of a path name, which should be stripped off from the
// map keys and function names.
//
// For example, running without the `-prefix` flag, we get:
//
// 	$ binpack /path/to/templates/
//
// 	_bindata["/path/to/templates/foo.html"] = path_to_templates_foo_html
//
// Running with the `-prefix` flag, we get:
//
// 	$ binpack -prefix "/path/to/" /path/to/templates/
//
// 	_bindata["templates/foo.html"] = templates_foo_html
//
// Build tags
//
// With the optional Tags field, you can specify any go build tags that must be
// fulfilled for the output file to be included in a build. This is useful when
// including binary data in multiple formats, where the desired format is
// specified at build time with the appropriate tags.
//
// The tags are appended to a `// +build` line in the beginning of the output
// file and must follow the build tags syntax specified by the go tool.
package binpack

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// Binpack handles bin packing assets.
type Binpack struct {
	// Package is the name of the package to use.
	Package string

	// Tags specify a set of optional build tags, which should be included in
	// the generated output. The tags are appended to a `// +build` line in the
	// beginning of the output file and must follow the build tags syntax
	// specified by the go tool.
	Tags string

	// Input defines the directory path, containing all asset files as well as
	// whether to recursively process assets in any sub directories.
	Input []Input

	// Output defines the output file for the generated code.  If left empty,
	// this defaults to 'bindata.go' in the current working directory.
	Output string

	// Prefix defines a path prefix which should be stripped from all file
	// names when generating the keys in the table of contents.  For example,
	// running without the `-prefix` flag, we get:
	//
	// 	$ binpack /path/to/templates
	// 	go_bindata["/path/to/templates/foo.html"] = _path_to_templates_foo_html
	//
	// Running with the `-prefix` flag, we get:
	//
	// 	$ binpack -prefix "/path/to/" /path/to/templates/foo.html
	// 	go_bindata["templates/foo.html"] = templates_foo_html
	Prefix string

	// NoMemCopy will alter the way the output file is generated.
	//
	// It will employ a hack that allows us to read the file data directly from
	// the compiled program's `.rodata` section. This ensures that when we call
	// call our generated function, we omit unnecessary mem copies.
	//
	// The downside of this, is that it requires dependencies on the `reflect`
	// and `unsafe` packages. These may be restricted on platforms like
	// AppEngine and thus prevent you from using this mode.
	//
	// Another disadvantage is that the byte slice we create, is strictly
	// read-only.  For most use-cases this is not a problem, but if you ever
	// try to alter the returned byte slice, a runtime panic is thrown. Use
	// this mode only on target platforms where memory constraints are an
	// issue.
	//
	// The default behaviour is to use the old code generation method. This
	// prevents the two previously mentioned issues, but will employ at least
	// one extra memcopy and thus increase memory requirements.
	//
	// For instance, consider the following two examples:
	//
	// This would be the default mode, using an extra memcopy but gives a safe
	// implementation without dependencies on `reflect` and `unsafe`:
	//
	// 	func myfile() []byte {
	// 		return []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a}
	// 	}
	//
	// Here is the same functionality, but uses the `.rodata` hack.
	// The byte slice returned from this example can not be written to without
	// generating a runtime error.
	//
	// 	var _myfile = "\x89\x50\x4e\x47\x0d\x0a\x1a"
	//
	// 	func myfile() []byte {
	// 		var empty [0]byte
	// 		sx := (*reflect.StringHeader)(unsafe.Pointer(&_myfile))
	// 		b := empty[:]
	// 		bx := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	// 		bx.Data = sx.Data
	// 		bx.Len = len(_myfile)
	// 		bx.Cap = bx.Len
	// 		return b
	// 	}
	NoMemCopy bool

	// NoCompress means the assets are /not/ GZIP compressed before being
	// turned into Go code. The generated function will automatically unzip the
	// file data when called. Defaults to false.
	NoCompress bool

	// Perform a debug build. This generates an asset file, which loads the
	// asset contents directly from disk at their original location, instead of
	// embedding the contents in the code.
	//
	// This is mostly useful if you anticipate that the assets are going to
	// change during your development cycle. You will always want your code to
	// access the latest version of the asset.  Only in release mode, will the
	// assets actually be embedded in the code. The default behaviour is
	// Release mode.
	Debug bool

	// Perform a dev build, which is nearly identical to the debug option. The
	// only difference is that instead of absolute file paths in generated
	// code, it expects a variable, `rootDir`, to be set in the generated
	// code's package (the author needs to do this manually), which it then
	// prepends to an asset's name to construct the file path on disk.
	//
	// This is mainly so you can push the generated code file to a shared
	// repository.
	Dev bool

	// When true, size, mode and modtime are not preserved from files
	NoMetadata bool

	// When nonzero, use this as mode for all files.
	Mode uint

	// When nonzero, use this as unix timestamp for all files.
	ModTime int64

	// Ignores any filenames matching the regex pattern specified, e.g.
	// path/to/file.ext will ignore only that file, or \\.gitignore will match
	// any .gitignore file.
	//
	// This parameter can be provided multiple times.
	Ignore []*regexp.Regexp
}

// Run processes the specified asset paths, converts them to Go code and writes
// new files to the output specified in the given configuration.
func (bp *Binpack) Run(out string, paths ...[]PathSpec) error {
	var err error

	// ensure we have sane values
	if err = bp.validate(); err != nil {
		return err
	}

	// get working directory
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	var toc []asset
	knownFuncs, visitedPaths := make(map[string]int), make(map[string]bool)

	// locate all assets
	for _, input := range inputs {
		if err = findFiles(input.Path, bp.Prefix, input.Recursive, &toc, bp.Ignore, knownFuncs, visitedPaths); err != nil {
			return err
		}
	}

	// create output
	w, err := os.Create(bp.Output)
	if err != nil {
		return err
	}
	defer w.Close()

	// buffer output (for working with large assets)
	buf := bufio.NewWriter(w)
	defer buf.Flush()

	// write build tags, if applicable
	if len(bp.Tags) > 0 {
		if _, err = buf.WriteString("// +build " + bp.Tags + "\n\n"); err != nil {
			return err
		}
	}

	// write package declaration
	if _, err = buf.WriteString("package " + bp.Package + "\n\n"); err != nil {
		return err
	}

	// write code generation header and sources
	if _, err = buf.WriteString("// Code generated by binpack. DO NOT EDIT.\n\n// sources:\n"); err != nil {
		return err
	}

	// add list of all assets
	for _, asset := range toc {
		relative, _ := filepath.Rel(wd, asset.Path)
		if _, err = fmt.Fprintf(buf, "// %s\n", filepath.ToSlash(relative)); err != nil {
			return err
		}
	}

	// determine if writing release or debug variant
	f := writeRelease
	if bp.Debug || bp.Dev {
		f = writeDebug
	}

	// write assets
	if err = f(buf, bp, toc); err != nil {
		return err
	}

	// write table of contents
	if err = writeTOC(buf, toc); err != nil {
		return err
	}

	// write toc tree
	if err = writeTOCTree(buf, toc); err != nil {
		return err
	}

	// write restore
	return writeRestore(buf)
}

// validate ensures the config has sane values.
// Part of which means checking if certain file/directory paths exist.
func (bp *Binpack) validate() error {
	if len(bp.Package) == 0 {
		return errors.New("missing package name")
	}

	for _, input := range bp.Input {
		_, err := os.Lstat(input.Path)
		if err != nil {
			return fmt.Errorf("failed to stat input path '%s': %v", input.Path, err)
		}
	}

	if len(bp.Output) == 0 {
		cwd, err := os.Getwd()
		if err != nil {
			return errors.New("unable to determine current working directory")
		}

		bp.Output = filepath.Join(cwd, "bindata.go")
	}

	fi, err := os.Lstat(bp.Output)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("Output path: %v", err)
		}

		// File does not exist. This is fine, just make
		// sure the directory it is to be in exists.
		dir, _ := filepath.Split(bp.Output)
		if dir != "" {
			err = os.MkdirAll(dir, 0744)
			if err != nil {
				return fmt.Errorf("Create output directory: %v", err)
			}
		}
	}

	if fi != nil && fi.IsDir() {
		return fmt.Errorf("Output path is a directory.")
	}

	return nil
}
