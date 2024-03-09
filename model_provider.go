package sdk

type Parser func(interface{}) error
type SpecParser func(SpecMap, interface{}) error

type Producer func() (chan []byte, chan error)
type ProducerProvider func(Parser, SpecParser) (Producer, error)

type Consumer func() (chan []byte, chan error, chan bool)
type ConsumerProvider func(Parser, SpecParser) (Consumer, error)

type Transformer func([]byte) ([]byte, error)
type TransformerProvider func(Parser, SpecParser) (Transformer, error)
