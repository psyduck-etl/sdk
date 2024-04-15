package sdk

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/zclconf/go-cty/cty"
)

type Spec struct {
	Name        string
	Description string
	Required    bool
	Type        cty.Type
	Default     cty.Value
}

type SpecMap map[string]*Spec

func itemSpec(source *Spec, key string, baseType *cty.Type) *Spec {
	name := strings.Join([]string{source.Name, key}, ".")
	if baseType == nil {
		panic(fmt.Sprintf("cannot gather element type of %s", name))
	}

	return &Spec{
		Name:        name,
		Description: source.Description,
		Required:    source.Required,
		Type:        *baseType,
		Default:     cty.NilVal,
	}
}

func ListItemSpec(source *Spec, index int) *Spec {
	return itemSpec(source, strconv.Itoa(index), cty.Type(source.Type).ListElementType())
}

func MapItemSpec(source *Spec, key string) *Spec {
	return itemSpec(source, key, cty.Type(source.Type).MapElementType())
}

func PipelineSpec() SpecMap {
	return SpecMap{
		"per-minute": &Spec{
			Name:        "per-minute",
			Description: "target producing/consuming n items per minute ( or 0 for unrestricted )",
			Type:        cty.Number,
			Required:    false,
			Default:     cty.NumberIntVal(180),
		},
	}
}
