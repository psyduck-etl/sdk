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
	ProvideProducer    ProducerProvider
	ProvideConsumer    ConsumerProvider
	ProvideTransformer TransformerProvider
}

type Plugin struct {
	Name      string
	Resources []*Resource
}

type Parser func(interface{}) error
type SpecParser func(SpecMap, interface{}) error

type Producer func() (chan []byte, chan error)
type ProducerProvider func(Parser, SpecParser) (Producer, error)

type Consumer func() (chan []byte, chan error, chan bool)
type ConsumerProvider func(Parser, SpecParser) (Consumer, error)

type Transformer func([]byte) ([]byte, error)
type TransformerProvider func(Parser, SpecParser) (Transformer, error)
