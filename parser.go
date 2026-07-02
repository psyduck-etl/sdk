package sdk

// Parser decodes a config block into dst. Its shape matches
// ConfigBlock.Decode so that a bound block.Decode value may be handed
// directly to a Provider.
type Parser func(dst any) error

// Provider is a plugin author's factory: given a Parser it returns a
// configured Producer, Consumer, or Transformer.
type Provider[T Producer | Consumer | Transformer] func(parse Parser) (T, error)

// Producer emits data onto send. It reports errors on errs. It is
// responsible for closing send when done producing.
type Producer func(send chan<- []byte, errs chan<- error)

// Consumer receives data from recv. It reports errors on errs and
// signals completion by sending on done.
type Consumer func(recv <-chan []byte, errs chan<- error, done chan<- struct{})

// Transformer maps one input datum to one output datum.
type Transformer func(in []byte) ([]byte, error)
