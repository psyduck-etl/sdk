package sdk

import (
	"github.com/zclconf/go-cty/cty"
)

type Spec struct {
	Name        string
	Description string
	Required    bool
	Type        cty.Type
	Default     cty.Value
}

var (
	SpecPerMinute = &Spec{
		Name:        "per-minute",
		Description: "target producing/consuming n items per minute ( or 0 for unrestricted )",
		Type:        cty.Number,
		Required:    false,
		Default:     cty.NumberIntVal(180),
	}
)
