package sdk

import (
	"github.com/zclconf/go-cty/cty"
)

type Type cty.Type

var (
	Bool    Type = Type(cty.Bool)
	String  Type = Type(cty.String)
	Integer Type = Type(cty.Number)
	Float   Type = Type(cty.Number)
)

func List(child Type) Type {
	return Type(cty.List(cty.Type(child)))
}

func Map(child Type) Type {
	return Type(cty.Map(cty.Type(child)))
}
