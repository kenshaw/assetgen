package gen

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/kenshaw/assetgen/gen/sigs"
	"github.com/kenshaw/assetgen/pack"
	"golang.org/x/crypto/openpgp"
)

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
		platform, ext = "win", ".zip"
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
		return "", "", fmt.Errorf("could not stat %q: %w", binPath, err)
	case fi.IsDir():
		return "", "", fmt.Errorf("%q is in invalid state: manually remove to try again", nodePath)
	case runtime.GOOS == "windows" || fi.Mode()|0111 != 0:
		return nodePath, binPath, nil
	}
	// remove existing directory
	if err = os.RemoveAll(nodePath); err != nil {
		return "", "", fmt.Errorf("could not remove %q: %w", nodePath, err)
	}
	// retrieve archive
	buf, err := getNodeAndVerify(flags, v, platform, ext)
	if err != nil {
		return "", "", fmt.Errorf("could not retrieve node %s (%s): %w", v, platform, err)
	}
	// extract archive
	if err = extractArchive(nodePath, buf, ext, fmt.Sprintf("node-%s-%s", v, platform)+"/"); err != nil {
		return "", "", fmt.Errorf("unable to extract node %s (%s): %w", v, platform, err)
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
		return "", fmt.Errorf("could not retrieve available node versions: %w", err)
	}
	// parse node versions
	var nodeVersions []nodeVersion
	if err = json.Unmarshal(verBuf, &nodeVersions); err != nil {
		return "", fmt.Errorf("node versions.json is invalid: %w", err)
	}
	if len(nodeVersions) < 1 {
		return "", errors.New("node versions.json missing a defined version")
	}
	// sort node versions
	vers, vs := make(map[string]nodeVersion), make([]*semver.Version, len(nodeVersions))
	for i, nv := range nodeVersions {
		v, err := semver.NewVersion(nv.Version)
		if err != nil {
			return "", fmt.Errorf("invalid node version %q: %w", nv.Version, err)
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
	kr, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(sigs.NodeJsPub))
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckDetachedSignature(kr, bytes.NewReader(txt), bytes.NewReader(sig))
	if err != nil {
		return nil, fmt.Errorf("could not verify signature: %w", err)
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
		return nil, fmt.Errorf("could not read SHASUMS256.txt: %w", err)
	}
	if !found {
		return nil, fmt.Errorf("could not find signature in SHASUMS256.txt for %s", fn)
	}
	return buf, nil
}

var semverRE = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+$`)

// installYarn installs yarn to the cache directory.
func installYarn(flags *Flags) (string, string, error) {
	v, assets, err := githubLatestAssets(flags, "yarnpkg/yarn", "yarn")
	if err != nil {
		return "", "", err
	}
	if !semverRE.MatchString(v) {
		return "", "", fmt.Errorf("cannot retrieve latest yarn release: invalid release tag %s", v)
	}
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
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
		return "", "", fmt.Errorf("could not stat %q: %w", binPath, err)
	case fi.IsDir():
		return "", "", fmt.Errorf("%q is in invalid state: manually remove to try again", yarnPath)
	case runtime.GOOS == "windows" || fi.Mode()|0111 != 0:
		return yarnPath, binPath, nil
	}
	// remove existing directory
	if err = os.RemoveAll(yarnPath); err != nil {
		return "", "", fmt.Errorf("could not remove %q: %w", yarnPath, err)
	}
	// retrieve archive
	buf, err := getYarnAndVerify(flags, v, assets)
	if err != nil {
		return "", "", fmt.Errorf("could not retrieve yarn %s: %w", v, err)
	}
	// create dir
	if err = os.MkdirAll(yarnPath, 0755); err != nil {
		return "", "", fmt.Errorf("could not create yarn %s directory: %w", v, err)
	}
	// extract archive
	if err = extractTarGz(yarnPath, buf, fmt.Sprintf("yarn-%s", v)); err != nil {
		return "", "", fmt.Errorf("unable to extract yarn %s: %w", v, err)
	}
	return yarnPath, binPath, nil
}

// getYarnAndVerify retrieves the yarn source distribution for the specified
// version, and verifies it against the accompanying .asc file.
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
	kr, err := openpgp.ReadArmoredKeyRing(bytes.NewReader(sigs.YarnPub))
	if err != nil {
		return nil, err
	}
	_, err = openpgp.CheckArmoredDetachedSignature(kr, bytes.NewReader(buf), bytes.NewReader(asc))
	if err != nil {
		return nil, fmt.Errorf("could not verify signature: %w", err)
	}
	return buf, nil
}

var webfontRE = regexp.MustCompile(`\.(woff|woff2|ttf|svg|eot)$`)

// installFontAwesome installs font awesome files.
func installFontAwesome(flags *Flags, dist *pack.Pack) error {
	v, assets, err := githubLatestAssets(flags, "FortAwesome/Font-Awesome", "fontawesome")
	if err != nil {
		return err
	}
	// check release name
	if !strings.HasPrefix(v, "Release ") {
		return fmt.Errorf("invalid fontawesome release %q", v)
	}
	v = strings.TrimPrefix(v, "Release ")
	// find asset
	n := fmt.Sprintf("fontawesome-free-%s-web", v)
	fn := n + ".zip"
	var asset githubAsset
	var found bool
	for _, a := range assets {
		if a.Name == fn {
			asset, found = a, true
			break
		}
	}
	if !found {
		return fmt.Errorf("could not find fontawesome asset %s for release %s", fn, v)
	}
	// retrieve release
	buf, err := getAndCache(flags, asset.BrowserDownloadURL, 0, false, "fontawesome", fn)
	if err != nil {
		return err
	}
	// remove and create build/fontawesome
	dir := filepath.Join(flags.Build, "fontawesome")
	if err = os.RemoveAll(dir); err != nil {
		return fmt.Errorf("could not remove fontawesome build directory: %w", err)
	}
	if err = os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("could not create fontawesome build directory: %w", err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		return err
	}
	// extract and process
	for _, z := range r.File {
		switch {
		case strings.HasPrefix(z.Name, n+"/scss/") && strings.HasSuffix(z.Name, ".scss"):
			fr, err := z.Open()
			if err != nil {
				return err
			}
			sbuf, err := ioutil.ReadAll(fr)
			if err != nil {
				return err
			}
			sbuf = bytes.Replace(sbuf, []byte("url("), []byte("asset("), -1)
			// prefix filename
			bn := filepath.Base(z.Name)
			if !strings.HasPrefix(bn, "_") && bn != "fontawesome.scss" {
				bn = "fontawesome-" + bn
			}
			out := filepath.Join(dir, bn)
			if err = ioutil.WriteFile(out, sbuf, 0644); err != nil {
				return fmt.Errorf("could not write fontawesome file %s: %w", out, err)
			}
			if err = fr.Close(); err != nil {
				return err
			}
		case strings.HasPrefix(z.Name, n+"/webfonts/") && webfontRE.MatchString(z.Name):
			fr, err := z.Open()
			if err != nil {
				return err
			}
			wbuf, err := ioutil.ReadAll(fr)
			if err != nil {
				return err
			}
			if err = dist.AddBytes("/webfonts/"+filepath.Base(z.Name), wbuf); err != nil {
				return err
			}
			if err = fr.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}
