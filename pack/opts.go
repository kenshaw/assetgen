package pack

// Option is a pack option.
type Option func(*Pack)

// WithPackageName is a pack option to specify the emitted Go package name.
func WithPackageName(pkg string) Option {
	return func(p *Pack) {
		p.pkg = pkg
	}
}
