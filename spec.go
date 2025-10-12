package sdk

import (
	"github.com/zclconf/go-cty/cty"
)

// Spec defines a configuration parameter for a plugin resource.
// Specs describe the type, requirements, and defaults for resource configuration fields.
type Spec struct {
	// Name is the parameter name used in configuration
	Name string
	
	// Description provides human-readable documentation for this parameter
	Description string
	
	// Required indicates whether this parameter must be provided
	Required bool
	
	// Type specifies the expected cty.Type for this parameter
	Type cty.Type
	
	// Default provides a default value if the parameter is not specified
	// Only used when Required is false
	Default cty.Value
}

// Common pre-defined specs that plugins can reuse
var (
	// SpecPerMinute defines a rate limiting parameter for producers/consumers
	SpecPerMinute = &Spec{
		Name:        "per-minute",
		Description: "target producing/consuming n items per minute (or 0 for unrestricted)",
		Type:        cty.Number,
		Required:    false,
		Default:     cty.NumberIntVal(180),
	}
	
	// SpecTimeout defines a timeout parameter for operations
	SpecTimeout = &Spec{
		Name:        "timeout",
		Description: "operation timeout in seconds",
		Type:        cty.Number,
		Required:    false,
		Default:     cty.NumberIntVal(30),
	}
	
	// SpecRetries defines a retry count parameter
	SpecRetries = &Spec{
		Name:        "retries",
		Description: "number of retry attempts on failure",
		Type:        cty.Number,
		Required:    false,
		Default:     cty.NumberIntVal(3),
	}
)
