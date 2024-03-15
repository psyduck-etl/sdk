package sdk

type Parser func(interface{}) error
type SpecParser func(SpecMap, interface{}) error

type Producefunc func() ([]byte, bool, error) // data-next, done
type Producer func() Producefunc
type ProducerProvider func(Parser, SpecParser) (Producer, error)

type Consumefunc func([]byte) error
type Consumer func() Consumefunc
type ConsumerProvider func(Parser, SpecParser) (Consumer, error)

type Transformer func([]byte) ([]byte, error)
type TransformerProvider func(Parser, SpecParser) (Transformer, error)
