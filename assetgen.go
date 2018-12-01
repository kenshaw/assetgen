// Package assetgen contains the assetgen util funcs.
package assetgen

var (
	// DefaultAssets are the default assets.
	DefaultAssets Assets
)

// Assets is the common interface for asset sets.
type Assets interface {
	// Asset returns the asset name.
	Asset(string) string
}

// AssetSet is a set of assets.
type AssetSet struct {
}

// NewAssetSet
func NewAssetSet() {

}

// Asset returns an asset path. For use in templates.
func Asset(name string) string {
	return DefaultAssets.Asset(name)
}
