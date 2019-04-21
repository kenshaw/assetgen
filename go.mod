module github.com/brankas/assetgen

go 1.12

require (
	github.com/Masterminds/semver v1.4.2
	github.com/gobwas/glob v0.2.3
	github.com/mattn/anko v0.1.2
	github.com/shurcooL/httpfs v0.0.0-20181222201310-74dc9339e414
	github.com/shurcooL/httpgzip v0.0.0-20180522190206-b1c53ac65af9
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	github.com/spf13/afero v1.2.2
	github.com/valyala/quicktemplate v1.1.0
	github.com/yookoala/realpath v1.0.0
	golang.org/x/crypto v0.0.0-20190418165655-df01cb2cc480
	golang.org/x/net v0.0.0-20190420063019-afa5a82059c6 // indirect
	golang.org/x/sync v0.0.0-20190412183630-56d357773e84
	golang.org/x/tools v0.0.0-20190420181800-aa740d480789 // indirect
)

replace github.com/shurcooL/vfsgen => github.com/kenshaw/vfsgen v0.0.0-20181201224209-11cc086c1a6d
