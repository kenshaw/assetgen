module github.com/brankas/assetgen

require (
	github.com/Masterminds/semver v1.4.2
	github.com/gobwas/glob v0.2.3
	github.com/mattn/anko v0.0.7
	github.com/shurcooL/httpfs v0.0.0-20181222201310-74dc9339e414
	github.com/shurcooL/httpgzip v0.0.0-20180522190206-b1c53ac65af9
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	github.com/spf13/afero v1.2.0
	github.com/valyala/quicktemplate v1.0.0
	github.com/yookoala/realpath v1.0.0
	golang.org/x/crypto v0.0.0-20190103213133-ff983b9c42bc
	golang.org/x/net v0.0.0-20190110200230-915654e7eabc // indirect
	golang.org/x/sync v0.0.0-20181221193216-37e7f081c4d4
	golang.org/x/tools v0.0.0-20190110211028-68c5ac90f574 // indirect
)

replace github.com/valyala/quicktemplate => github.com/kenshaw/quicktemplate v0.0.0-20181201010149-180468dad8e9

replace github.com/shurcooL/vfsgen => github.com/kenshaw/vfsgen v0.0.0-20181201224209-11cc086c1a6d
