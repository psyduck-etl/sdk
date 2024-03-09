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
	Type        Type
	Default     cty.Value
}

type SpecMap map[string]*Spec

func ListItemSpec(source *Spec, index int) *Spec {
	return itemSpec(source, strconv.Itoa(index), cty.Type(source.Type).ListElementType())
}

func MapItemSpec(source *Spec, key string) *Spec {
	return itemSpec(source, key, cty.Type(source.Type).MapElementType())
}
func itemSpec(source *Spec, key string, baseType *cty.Type) *Spec {
	name := strings.Join([]string{source.Name, key}, ".")
	if baseType == nil {
		panic(fmt.Sprintf("cannot gather element type of %s", name))
	}

	return &Spec{
		Name:        name,
		Description: source.Description,
		Required:    source.Required,
		Type:        Type(*baseType),
		Default:     cty.NilVal,
	}
}
