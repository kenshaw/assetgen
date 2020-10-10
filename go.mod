module github.com/kenshaw/assetgen

go 1.12

require (
	github.com/Masterminds/semver v1.5.0
	github.com/gobwas/glob v0.2.3
	github.com/mattn/anko v0.1.8
	github.com/shurcooL/httpfs v0.0.0-20190707220628-8d4bc4ba7749
	github.com/shurcooL/httpgzip v0.0.0-20190720172056-320755c1c1b0
	github.com/shurcooL/vfsgen v0.0.0-20200824052919-0d455de96546
	github.com/spf13/afero v1.4.1
	github.com/valyala/quicktemplate v1.6.3
	github.com/yookoala/realpath v1.0.0
	golang.org/x/crypto v0.0.0-20201002170205-7f63de1d35b0
	golang.org/x/net v0.0.0-20201006153459-a7d1128ccaa0 // indirect
	golang.org/x/sync v0.0.0-20200930132711-30421366ff76
	golang.org/x/tools v0.0.0-20190420181800-aa740d480789 // indirect
)

replace github.com/shurcooL/vfsgen => github.com/kenshaw/vfsgen v0.1.0
