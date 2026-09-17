package sketchtest

import "github.com/lestrrat-go/option/v3"

// Option configures a numeric comparison.
type Option interface {
	option.Interface
	sketchtestOption()
}

type sketchtestOption struct{ option.Interface }

func (sketchtestOption) sketchtestOption() {}

type identWithin struct{}
type identWithinRel struct{}

// Within sets an absolute tolerance for a numeric comparison.
func Within(abs float64) Option {
	return sketchtestOption{option.New(identWithin{}, abs)}
}

// WithinRel sets a tolerance equal to rel times the absolute expected value.
func WithinRel(rel float64) Option {
	return sketchtestOption{option.New(identWithinRel{}, rel)}
}

type comparisonConfig struct {
	value     float64
	relative  bool
	set       bool
	nilOption bool
}

func resolveOptions(opts []Option) comparisonConfig {
	var cfg comparisonConfig
	for _, opt := range opts {
		if opt == nil {
			cfg.nilOption = true
			continue
		}
		switch opt.Ident().(type) {
		case identWithin:
			if value, ok := option.Get[float64](opt); ok {
				cfg.value = value
				cfg.relative = false
				cfg.set = true
			}
		case identWithinRel:
			if value, ok := option.Get[float64](opt); ok {
				cfg.value = value
				cfg.relative = true
				cfg.set = true
			}
		}
	}
	return cfg
}
