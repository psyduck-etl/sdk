package sdk

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type MoverKind uint64

const (
	PRODUCER = MoverKind(1 << iota)
	CONSUMER
	TRANSFORMER
)

type Resource struct {
	Kinds              MoverKind
	Name               string
	Spec               []*Spec
	ProvideProducer    Provider[Producer]
	ProvideConsumer    Provider[Consumer]
	ProvideTransformer Provider[Transformer]
	specMap            map[string]*Spec
}

type Plugin struct {
	Name      string
	Resources []*Resource
	Variables map[string]cty.Value
	Functions map[string]function.Function
}

func genFunc(res *Resource) function.Function {
	params := make([]function.Parameter, len(res.Spec))
	for i, spec := range res.Spec {
		params[i] = function.Parameter{
			Name:             spec.Name,
			Description:      spec.Description,
			Type:             spec.Type,
			AllowNull:        false,
			AllowUnknown:     false,
			AllowDynamicType: false,
			AllowMarked:      false,
		}
	}

	return function.New(&function.Spec{
		Description: "Generate a " + res.Name + " resource",
		Params:      params,
		Type: func(args []cty.Value) (cty.Type, error) {
			options := make(map[string]cty.Type, len(args))
			for i := range len(args) {
				options[params[i].Name] = params[i].Type
			}

			return cty.Object(map[string]cty.Type{
				"resource": cty.String,
				"options":  cty.Object(options),
			}), nil
		},
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			options := make(map[string]cty.Value, len(args))
			for i, arg := range args {
				options[params[i].Name] = arg
			}

			return cty.ObjectVal(map[string]cty.Value{
				"resource": cty.StringVal(res.Name),
				"options":  cty.ObjectVal(options),
			}), nil
		},
		// TODO RefineResult based on spec defaults / requireds?
	})
}

func (p *Resource) SpecMap() map[string]*Spec {
	if p.specMap == nil {
		p.specMap = make(map[string]*Spec, len(p.Spec))
		for _, spec := range p.Spec {
			p.specMap[spec.Name] = spec
		}
	}

	return p.specMap
}

func (p *Plugin) Ctx() *hcl.EvalContext {
	functions := make(map[string]function.Function, len(p.Resources))
	for _, res := range p.Resources {
		functions[res.Name] = genFunc(res)
	}

	return &hcl.EvalContext{
		Variables: map[string]cty.Value{},
		Functions: functions,
	}
}

type Parser func(interface{}) error

type Provider[T Producer | Consumer | Transformer] func(parse Parser) (T, error)
type Producer func(send chan<- []byte, errs chan<- error)
type Consumer func(recv <-chan []byte, errs chan<- error, done chan<- struct{})
type Transformer func(in []byte) ([]byte, error)
