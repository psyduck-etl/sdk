package sdk

type kind int

const (
	PRODUCER    kind = 0b0001
	CONSUMER    kind = 0b0010
	TRANSFORMER kind = 0b0100
)

type Resource struct {
	Kinds              kind
	Name               string
	Spec               SpecMap
	ProvideProducer    Provider[Producer]
	ProvideConsumer    Provider[Consumer]
	ProvideTransformer Provider[Transformer]
}

type Plugin struct {
	Name      string
	Resources []*Resource
}

type Parser func(interface{}) error
type SpecParser func(SpecMap, interface{}) error

type Provider[T any] func(Parser, SpecParser) (T, error)
type Producer func(send chan<- []byte, errs chan<- error)
type Consumer func(recv <-chan []byte, errs chan<- error, done chan<- struct{})
type Transformer func(in []byte) ([]byte, error)
