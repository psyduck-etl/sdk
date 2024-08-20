package sdk

import (
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

type MoverKind uint64

const (
	PRODUCER = MoverKind(1 << iota)
	CONSUMER
	TRANSFORMER
)

type MoverDesc struct {
	Kind     MoverKind
	Resource string    `hcl:"resource" cty:"resource"`
	Options  cty.Value `hcl:",remain" cty:"options"`
}

type Resource struct {
	Kinds              MoverKind
	Name               string
	Spec               SpecMap
	ProvideProducer    Provider[Producer]
	ProvideConsumer    Provider[Consumer]
	ProvideTransformer Provider[Transformer]
}

type Plugin struct {
	Name      string
	Resources []*Resource
	Functions map[string]function.Function
}

type Parser func(interface{}) error

type Provider[T Producer | Consumer | Transformer] func(parse Parser) (T, error)
type Producer func(send chan<- []byte, errs chan<- error)
type Consumer func(recv <-chan []byte, errs chan<- error, done chan<- struct{})
type Transformer func(in []byte) ([]byte, error)
