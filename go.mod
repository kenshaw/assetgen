module github.com/brankas/assetgen

require (
	github.com/Masterminds/semver v1.4.2
	github.com/klauspost/compress v1.4.1 // indirect
	github.com/klauspost/cpuid v1.2.0 // indirect
	github.com/mattn/anko v0.0.6
	github.com/shurcooL/httpfs v0.0.0-20171119174359-809beceb2371
	github.com/shurcooL/httpgzip v0.0.0-20180522190206-b1c53ac65af9
	github.com/shurcooL/vfsgen v0.0.0-20181202132449-6a9ea43bcacd
	github.com/spf13/afero v1.1.2
	github.com/valyala/quicktemplate v1.0.0
	github.com/yookoala/realpath v1.0.0
	golang.org/x/crypto v0.0.0-20181127143415-eb0de9b17e85 // indirect
	golang.org/x/net v0.0.0-20181201002055-351d144fa1fc // indirect
	golang.org/x/sync v0.0.0-20181108010431-42b317875d0f
	golang.org/x/text v0.3.0 // indirect
	golang.org/x/tools v0.0.0-20181201035826-d0ca3933b724
	gopkg.in/check.v1 v1.0.0-20180628173108-788fd7840127 // indirect
)

replace github.com/valyala/quicktemplate => github.com/kenshaw/quicktemplate v0.0.0-20181201010149-180468dad8e9

replace github.com/shurcooL/vfsgen => github.com/kenshaw/vfsgen v0.0.0-20181201224209-11cc086c1a6d
