package gen

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/yookoala/realpath"
	"golang.org/x/crypto/openpgp"
)

const (
	nodeConstraint = ">=10.15.x"
	yarnConstraint = ">=1.12.x"

	yarnRcName = ".yarnrc"

	cacheDir          = ".cache"
	buildDir          = "build"
	nodeModulesDir    = "node_modules"
	nodeModulesBinDir = ".bin"
	assetsDir         = "assets"

	productionEnv  = "production"
	developmentEnv = "development"

	scriptName   = "assets.anko"
	assetsFile   = "assets.go"
	fontsDir     = "fonts"
	geoipDir     = "geoip"
	localesDir   = "locales"
	imagesDir    = "images"
	jsDir        = "js"
	sassDir      = "sass"
	cssDir       = "css"
	sassJs       = "sass.js"
	templatesDir = "templates"

	nodeDistURL = "https://nodejs.org/dist"
	geoipURL    = "https://geolite.maxmind.com/download/geoip/database/GeoLite2-Country.mmdb.gz"
)

// Run generates assets using the current working directory and default flags.
func Run() error {
	var err error

	// load working directory
	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("could not determine working directory: %v", err)
	}

	// build flags
	flags := NewFlags(wd)
	fs := flags.FlagSet(filepath.Base(os.Args[0]), flag.ExitOnError)
	err = fs.Parse(os.Args[1:])
	if err != nil {
		return fmt.Errorf("could not parse args: %v", err)
	}

	return Assetgen(flags)
}

// Assetgen generates assets based on the passed flags.
func Assetgen(flags *Flags) error {
	var err error

	// check working directory is usable
	wdfi, err := os.Stat(flags.Wd)
	if err != nil || !wdfi.IsDir() {
		return fmt.Errorf("cannot read from working directory %q", flags.Wd)
	}
	wd, err := realpath.Realpath(flags.Wd)
	if err != nil {
		return fmt.Errorf("could not determine real path for %s: %v", flags.Wd, err)
	}
	flags.Wd = wd

	// ensure workers is at least 1
	if flags.Workers < 1 {
		return errors.New("workers must be at least 1")
	}

	// ensure valid trans func name
	if !isValidIdentifier(flags.TFuncName) {
		return errors.New("invalid trans func name")
	}

	// ensure paths are set
	if flags.Cache == "" {
		flags.Cache = filepath.Join(flags.Wd, cacheDir)
	}
	if flags.Build == "" {
		flags.Build = filepath.Join(flags.Wd, buildDir)
	}
	if flags.NodeModules == "" {
		flags.NodeModules = filepath.Join(flags.Cache, nodeModulesDir)
	}
	if flags.NodeModulesBin == "" {
		flags.NodeModulesBin = filepath.Join(flags.NodeModules, nodeModulesBinDir)
	}
	if flags.Assets == "" {
		flags.Assets = filepath.Join(flags.Wd, assetsDir)
	}
	if flags.Script == "" {
		flags.Script = filepath.Join(flags.Assets, scriptName)
	}

	// set working directory
	if err = os.Chdir(flags.Wd); err != nil {
		return fmt.Errorf("could not change to dir: %v", err)
	}

	// check setup
	if err = checkSetup(flags); err != nil {
		return err
	}

	// set PATH
	if err = os.Setenv("PATH", strings.Join([]string{
		flags.Node,
		flags.NodeModulesBin,
		os.Getenv("PATH"),
	}, ":")); err != nil {
		return fmt.Errorf("could not set PATH: %v", err)
	}

	// set NODE_PATH
	if err = os.Setenv("NODE_PATH", flags.NodeModules); err != nil {
		return fmt.Errorf("could not set NODE_PATH: %v", err)
	}
	// load script
	s, err := LoadScript(flags)
	if err != nil {
		return fmt.Errorf("unable to load script %s: %v", flags.Script, err)
	}

	// setup dependencies
	if err = s.ConfigDeps(); err != nil {
		return fmt.Errorf("unable to configure dependencies: %v", err)
	}

	// fix links in node/.bin directory
	if err = fixNodeModulesBinLinks(flags); err != nil {
		return fmt.Errorf("unable to fix bin links in %s: %v", flags.NodeModulesBin, err)
	}

	ctxt, cancel := context.WithCancel(context.Background())

	// start callback server
	sock, err := s.startCallbackServer(ctxt)
	if err != nil {
		return fmt.Errorf("could not start callback server: %v", err)
	}
	defer func() {
		cancel()
		if err := os.RemoveAll(filepath.Dir(sock)); err != nil {
			warnf(flags, "could not remove %s: %v", sock, err)
		}
	}()

	// set ASSETGEN_SOCK
	if err = os.Setenv("ASSETGEN_SOCK", sock); err != nil {
		return fmt.Errorf("could not set ASSETGEN_SOCK: %v", err)
	}

	// run script
	if err = s.Execute(); err != nil {
		return fmt.Errorf("could not run script: %v", err)
	}

	return nil
}

// checkSetup checks that yarn is the correct version, and all necessary files
// and directories exist as expected.
func checkSetup(flags *Flags) error {
	var err error

	// ensure primary directories exist
	if err = checkDirs(flags, &flags.Cache, &flags.Build, &flags.Assets); err != nil {
		return fmt.Errorf("could not create directories: %v", err)
	}

	// check node + yarn
	if err = checkNode(flags); err != nil {
		return err
	}
	if err = os.Setenv("PATH", filepath.Dir(flags.NodeBin)+":"+os.Getenv("PATH")); err != nil {
		return err
	}
	if err = checkYarn(flags); err != nil {
		return err
	}

	// ensure node_modules and assets directories exist
	for _, d := range []struct{ n, v string }{
		{"node_modules", flags.NodeModules},
		{"assets", flags.Assets},
	} {
		_, err := filepath.Rel(flags.Wd, d.v)
		if err != nil || !isParentDir(flags.Wd, d.v) {
			return fmt.Errorf("%s path must be subdirectory of working directory", d.n)
		}
	}

	var nodeModulesPresent bool
	_, err = os.Stat(flags.NodeModules)
	switch {
	case err == nil:
		nodeModulesPresent = true
	}

	var yarnLockPresent bool
	_, err = os.Stat(filepath.Join(flags.Wd, "yarn.lock"))
	switch {
	case err == nil:
		yarnLockPresent = true
	}

	// setup files
	if err = setupFiles(flags); err != nil {
		return fmt.Errorf("unable to setup files: %v", err)
	}

	// do pure lockfile install
	if !nodeModulesPresent && yarnLockPresent {
		if err = run(flags, flags.YarnBin, "install", "--pure-lockfile"); err != nil {
			return errors.New("unable to install locked deps: please run yarn manually")
		}
	}

	// run yarn check
	if err = runSilent(flags, flags.YarnBin, "check"); err != nil {
		return errors.New("yarn is out of sync: please run yarn manually")
	}

	return nil
}

// checkDirs creates required directories and ensures node and assets are
// subdirectories of the working directory.
func checkDirs(flags *Flags, dirs ...*string) error {
	// make required directories
	for _, d := range dirs {
		v, err := filepath.Abs(*d)
		if err != nil {
			return fmt.Errorf("could not resolve path %q", *d)
		}
		err = os.MkdirAll(v, 0755)
		if err != nil {
			return fmt.Errorf("could not create directory %s: %v", v, err)
		}
		v, err = realpath.Realpath(v)
		if err != nil {
			return fmt.Errorf("could not determine realpath for %q", *d)
		}
		*d = v
	}

	return nil
}

// checkNode checks that node is available and the correct version.
//
// If node is not available, then the latest version is downloaded to the cache
// dir and used instead.
func checkNode(flags *Flags) error {
	var err error

	if flags.Node == "" {
		if flags.Node, flags.NodeBin, err = installNode(flags); err != nil {
			return err
		}
	}

	node, err := realpath.Realpath(flags.Node)
	if err != nil {
		return err
	}
	flags.Node = node

	if flags.NodeBin == "" {
		if runtime.GOOS == "windows" {
			flags.NodeBin = filepath.Join(flags.Node, "node.exe")
		} else {
			flags.NodeBin = filepath.Join(flags.Node, "bin", "node")
		}
	}

	// check node version
	nodeVer, err := runCombined(flags, flags.NodeBin, "--version")
	if err != nil {
		return fmt.Errorf("unable to determine node version: %v", err)
	}
	if !compareSemver(nodeVer, nodeConstraint) {
		return fmt.Errorf("%s version must be %s, currently: %s", flags.NodeBin, nodeConstraint, nodeVer)
	}

	return nil
}

// installNode installs node to the cache directory.
func installNode(flags *Flags) (string, string, error) {
	// get version
	v, err := getNodeLtsVersion(flags)
	if err != nil {
		return "", "", err
	}

	// env variables
	platform, ext := runtime.GOOS, ".tar.gz"
	switch runtime.GOOS {
	case "linux", "darwin":
	case "windows":
		platform, ext = "win", "zip"
	default:
		return "", "", fmt.Errorf("unsupported os: %s", runtime.GOOS)
	}
	platform += "-x64"

	// build paths
	nodePath := filepath.Join(flags.Cache, "node", v, platform)
	binPath := filepath.Join(nodePath, "bin", "node")
	if runtime.GOOS == "windows" {
		binPath = filepath.Join(nodePath, "node.exe")
	}

	// stat node path
	fi, err := os.Stat(binPath)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return "", "", fmt.Errorf("could not stat %q: %v", binPath, err)
	case fi.IsDir():
		return "", "", fmt.Errorf("%q is in invalid state: manually remove to try again", nodePath)
	case runtime.GOOS == "windows" || fi.Mode()|0111 != 0:
		return nodePath, binPath, nil
	}

	// remove existing directory
	if err = os.RemoveAll(nodePath); err != nil {
		return "", "", fmt.Errorf("could not remove %q: %v", nodePath, err)
	}

	// retrieve archive
	buf, err := getNodeAndVerify(flags, v, platform, ext)
	if err != nil {
		return "", "", fmt.Errorf("could not retrieve node %s (%s): %v", v, platform, err)
	}

	// extract archive
	if err = extractArchive(nodePath, buf, ext, fmt.Sprintf("node-%s-%s", v, platform)+"/"); err != nil {
		return "", "", fmt.Errorf("unable to extract node %s (%s): %v", v, platform, err)
	}

	return nodePath, binPath, nil
}

// ltsString is a type that handles unmarshaling the lts version in node's
// versions file.
type ltsString string

func (v *ltsString) UnmarshalJSON(buf []byte) error {
	var s string
	if err := json.Unmarshal(buf, &s); err == nil {
		*v = ltsString(s)
	}
	return nil
}

// getNodeLtsVersion reads the available node versions and returns the most
// recent lts release.
func getNodeLtsVersion(flags *Flags) (string, error) {
	type nodeVersion struct {
		Version string
		Files   []string
		Lts     ltsString
	}

	// load available node versions
	verBuf, err := getAndCache(flags, nodeDistURL+"/index.json", flags.Ttl, false, "node", "versions.json")
	if err != nil {
		return "", fmt.Errorf("could not retrieve available node versions: %v", err)
	}

	// parse node versions
	var nodeVersions []nodeVersion
	if err = json.Unmarshal(verBuf, &nodeVersions); err != nil {
		return "", fmt.Errorf("node versions.json is invalid: %v", err)
	}
	if len(nodeVersions) < 1 {
		return "", errors.New("node versions.json missing a defined version")
	}

	// sort node versions
	vers, vs := make(map[string]nodeVersion), make([]*semver.Version, len(nodeVersions))
	for i, nv := range nodeVersions {
		v, err := semver.NewVersion(nv.Version)
		if err != nil {
			return "", fmt.Errorf("invalid node version %q: %v", nv.Version, err)
		}
		vers[v.String()], vs[i] = nv, v
	}
	sort.Sort(semver.Collection(vs))

	// find latest lts
	for i := len(vs) - 1; i >= 0; i-- {
		if v := vers[vs[i].String()]; v.Lts != "" {
			return v.Version, nil
		}
	}

	return "", errors.New("could not find a lts node version")
}

// getNodeAndVerify retrieves the node.js binary distribution for the specified
// version, platform, and file extension and verifies its hash in the
// SHASUMS256.txt file.
func getNodeAndVerify(flags *Flags, version, platform, ext string) ([]byte, error) {
	fn := fmt.Sprintf("node-%v-%s%s", version, platform, ext)
	urlbase := nodeDistURL + "/" + version

	// grab signature files
	txt, err := getAndCache(flags, urlbase+"/SHASUMS256.txt", 0, false, "node", version, "SHASUMS256.txt")
	if err != nil {
		return nil, err
	}
	sig, err := getAndCache(flags, urlbase+"/SHASUMS256.txt.sig", 0, false, "node", version, "SHASUMS256.txt.sig")
	if err != nil {
		return nil, err
	}

	// verify signature
	kr, err := openpgp.ReadArmoredKeyRing(bytes.NewReader([]byte(nodeKeyring)))
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckDetachedSignature(kr, bytes.NewReader(txt), bytes.NewReader(sig))
	if err != nil {
		return nil, fmt.Errorf("could not verify signature: %v", err)
	}

	// get node
	buf, err := getAndCache(flags, urlbase+"/"+fn, 0, false, "node", fn)
	if err != nil {
		return nil, err
	}

	// verify hash
	h := sha256.Sum256(buf)
	hash := hex.EncodeToString(h[:])
	scanner := bufio.NewScanner(bytes.NewReader(txt))
	var found bool
	for scanner.Scan() {
		line := strings.Split(scanner.Text(), "  ")
		if len(line) != 2 {
			return nil, errors.New("SHASUMS256.txt is invalid")
		}
		found = found || (line[0] == hash && line[1] == fn)
	}
	if err = scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not read SHASUMS256.txt: %v", err)
	}

	if !found {
		return nil, fmt.Errorf("could not find signature in SHASUMS256.txt for %s", fn)
	}

	return buf, nil
}

// checkYarn checks that yarn is available and the correct version.
//
// If yarn is not available, then the latest version is downloaded to the cache
// dir and used instead.
func checkYarn(flags *Flags) error {
	var err error

	if flags.Yarn == "" {
		if flags.Yarn, flags.YarnBin, err = installYarn(flags); err != nil {
			return err
		}
	}

	yarn, err := realpath.Realpath(flags.Yarn)
	if err != nil {
		return err
	}
	flags.Yarn = yarn

	if flags.YarnBin == "" {
		if runtime.GOOS == "windows" {
			flags.YarnBin = filepath.Join(flags.Yarn, "bin", "yarn.cmd")
		} else {
			flags.YarnBin = filepath.Join(flags.Yarn, "bin", "yarn")
		}
	}

	// check yarn version
	yarnVer, err := runCombined(flags, flags.YarnBin, "--version")
	if err != nil {
		return fmt.Errorf("unable to determine yarn version: %v", err)
	}
	if !compareSemver(strings.TrimPrefix(yarnVer, "v"), yarnConstraint) {
		return fmt.Errorf("%s version must be %s, currently: %s", flags.YarnBin, yarnConstraint, yarnVer)
	}
	return nil
}

// installYarn installs yarn to the cache directory.
func installYarn(flags *Flags) (string, string, error) {
	v, assets, err := githubLatestAssets(flags, "yarnpkg/yarn", "yarn")
	if err != nil {
		return "", "", err
	}

	// build paths
	yarnPath := filepath.Join(flags.Cache, "yarn", v)
	binPath := filepath.Join(yarnPath, "bin", "yarn")
	if runtime.GOOS == "windows" {
		binPath = filepath.Join(yarnPath, "bin", "yarn.cmd")
	}

	// stat yarn path
	fi, err := os.Stat(binPath)
	switch {
	case os.IsNotExist(err):
	case err != nil:
		return "", "", fmt.Errorf("could not stat %q: %v", binPath, err)
	case fi.IsDir():
		return "", "", fmt.Errorf("%q is in invalid state: manually remove to try again", yarnPath)
	case runtime.GOOS == "windows" || fi.Mode()|0111 != 0:
		return yarnPath, binPath, nil
	}

	// remove existing directory
	if err = os.RemoveAll(yarnPath); err != nil {
		return "", "", fmt.Errorf("could not remove %q: %v", yarnPath, err)
	}

	// retrieve archive
	buf, err := getYarnAndVerify(flags, v, assets)
	if err != nil {
		return "", "", fmt.Errorf("could not retrieve yarn %s: %v", v, err)
	}

	// create dir
	if err = os.MkdirAll(yarnPath, 0755); err != nil {
		return "", "", fmt.Errorf("could not create yarn %s directory: %v", v, err)
	}

	// extract archive
	if err = extractTarGz(yarnPath, buf, fmt.Sprintf("yarn-%s", v)); err != nil {
		return "", "", fmt.Errorf("unable to extract yarn %s: %v", v, err)
	}

	return yarnPath, binPath, nil
}

// getYarnAndVerify retrieves the node.js binary distribution for the specified
// version, platform, and file extension and verifies its hash in the
// SHASUMS256.txt file.
func getYarnAndVerify(flags *Flags, version string, assets []githubAsset) ([]byte, error) {
	n := fmt.Sprintf("yarn-%v.tar.gz", version)

	var err error
	var buf, asc []byte
	for _, a := range assets {
		switch {
		// grab tar.gz
		case a.Name == n:
			buf, err = getAndCache(flags, a.BrowserDownloadURL, 0, false, "yarn", n)
			if err != nil {
				return nil, err
			}

		// grab signature
		case a.Name == n+".asc":
			asc, err = getAndCache(flags, a.BrowserDownloadURL, 0, false, "yarn", n+".asc")
			if err != nil {
				return nil, err
			}
		}
	}

	// verify signature
	kr, err := openpgp.ReadArmoredKeyRing(bytes.NewReader([]byte(yarnKeyring)))
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckArmoredDetachedSignature(kr, bytes.NewReader(buf), bytes.NewReader(asc))
	if err != nil {
		return nil, fmt.Errorf("could not verify signature: %v", err)
	}

	return buf, nil
}

// fixNodeModulesBinLinks walks all packages in flags.NodeModules, reading their bin entries from
// package.json, and creating the appropriate symlink in flags.NodeModulesBin.
func fixNodeModulesBinLinks(flags *Flags) error {
	var err error

	// check dirs
	if err = checkDirs(flags, &flags.NodeModulesBin); err != nil {
		return fmt.Errorf("unable to fix node bin directory: %v", err)
	}

	// erase all links in bin dir
	err = filepath.Walk(flags.NodeModulesBin, func(path string, fi os.FileInfo, err error) error {
		switch {
		case err != nil:
			return err
		case path == flags.NodeModulesBin:
			return nil
		case fi.Mode()&os.ModeSymlink == 0:
			return fmt.Errorf("%s is not a symlink", path)
		}
		err = os.Remove(path)
		if err != nil {
			return fmt.Errorf("unable to remove %s: %v", path, err)
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
		if err != nil {
			return err
		}
		if fi.IsDir() || filepath.Base(path) != "package.json" {
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
		err = json.Unmarshal(buf, &pkgDesc)
		if err != nil {
			warnf(flags, "could not unmarshal %s: %v", path, err)
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
				return fmt.Errorf("could not determine node-relative path for %s: %v", z.dir, err)
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
			return fmt.Errorf("unable to determine path for %s: %v", linkpath, err)
		}
		newname := filepath.Join(flags.NodeModulesBin, n)

		// check symlink exists
		_, err = os.Stat(newname)
		switch {
		case err != nil && os.IsNotExist(err):
		case err != nil:
			return err
		}

		// symlink
		if err = os.Symlink(oldname, newname); err != nil {
			return fmt.Errorf("unable to symlink %s to %s: %v", newname, oldname, err)
		}

		// fix permissions
		if runtime.GOOS != "windows" {
			err = os.Chmod(linkpath, 0755)
			if err != nil {
				return err
			}
		}
	}

	return nil
}
