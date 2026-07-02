package sdk

// SpecType enumerates the value shapes an sdk config field can take. The
// host's config-format code is responsible for translating its own type
// system (cty, YAML nodes, etc.) into these SDK-owned shapes.
type SpecType int

const (
	TypeString SpecType = iota + 1
	TypeInt
	TypeFloat
	TypeBool
	TypeList
	TypeMap
	TypeObject
)

// String returns a human-readable name for the SpecType. Unknown values
// render as "unknown".
func (t SpecType) String() string {
	switch t {
	case TypeString:
		return "string"
	case TypeInt:
		return "int"
	case TypeFloat:
		return "float"
	case TypeBool:
		return "bool"
	case TypeList:
		return "list"
	case TypeMap:
		return "map"
	case TypeObject:
		return "object"
	default:
		return "unknown"
	}
}

// Spec describes one configuration field a resource accepts. Nested types
// use ElemType (List/Map element type) or Fields (Object attributes).
// Default is a Go-native value; the host format converts it to its own
// representation.
type Spec struct {
	Name        string
	Description string
	Required    bool
	Type        SpecType
	ElemType    *Spec
	Fields      []*Spec
	Default     any
}
