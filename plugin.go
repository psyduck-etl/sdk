package sdk

// Kind is a bitmask of resource capabilities. A resource may advertise more
// than one Kind by OR-ing the values together (e.g. PRODUCER|CONSUMER).
type Kind uint64

const (
	PRODUCER Kind = 1 << iota
	CONSUMER
	TRANSFORMER
)

// Plugin is what the host sees. In-process implementations are assembled by
// NewInProc; future RPC/socket implementations satisfy this interface
// directly.
type Plugin interface {
	// Name returns the plugin's identifier.
	Name() string
	// Resources lists every resource this plugin offers, as metadata only.
	Resources() []ResourceDescriptor
	// Bind configures a resource of the given kind, using block for decoding
	// resource-specific options. The returned Instance is ready to run.
	Bind(kind Kind, resource string, block ConfigBlock) (Instance, error)
}

// ResourceDescriptor is host-visible metadata about a resource. It carries
// no callable code — the plugin retains ownership of the underlying
// provider closures.
type ResourceDescriptor struct {
	Name  string
	Kinds Kind
	Spec  []*Spec
}

// Instance is a configured, ready-to-run resource. Kind reports which of
// Produce/Consume/Transform is meaningful for this instance. Calling a
// method not covered by Kind panics with a clear message — it is a
// programmer error.
type Instance interface {
	Kind() Kind
	Produce(send chan<- []byte, errs chan<- error)
	Consume(recv <-chan []byte, errs chan<- error, done chan<- struct{})
	Transform(in []byte) ([]byte, error)
	Close() error
}
