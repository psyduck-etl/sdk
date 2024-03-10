package sdk

import (
	"github.com/zclconf/go-cty/cty"
)

func SpecStopAfter(value int64) *Spec {
	return &Spec{
		Name:        "stop-after",
		Description: "Stop producing after n values have been produced ( or 0 for unlimited )",
		Type:        Integer,
		Required:    false,
		Default:     cty.NumberIntVal(value),
	}
}

func SpecPerMinute(value int64) *Spec {
	return &Spec{
		Name:        "per-minute",
		Description: "target producing/consuming n items per minute ( or 0 for unrestricted )",
		Type:        Integer,
		Required:    false,
		Default:     cty.NumberIntVal(value),
	}
}

func SpecExitOnError(value bool) *Spec {
	return &Spec{
		Name:        "exit-on-error",
		Description: "stop producing/consuming if we encounter an error",
		Type:        Bool,
		Required:    false,
		Default:     cty.BoolVal(value),
	}
}
