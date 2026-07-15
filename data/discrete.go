package data

import "sort"

// Discrete is the struct-like half of the model: data referenced by key (a
// JSON object, an HCL body). K is almost always string and values are
// heterogeneous, so the mainline interface is non-generic — Get returns the
// Value interface. Homogeneous, statically-typed containers use Map[K,V].
type Discrete interface {
	Value
	// Keys returns the object's keys in sorted order.
	Keys() []string
	// Get looks a key up. ok=false when the key is absent.
	Get(key string) (Value, bool)
}

// Object is the mainline Discrete: a keyed map of Values.
type Object map[string]Value

func (o Object) Kind() Kind    { return KindObject }
func (o Object) Bytes() []byte { return marshalJSON(o) }

func (o Object) String() string { return string(o.Bytes()) }

func (o Object) Keys() []string {
	keys := make([]string, 0, len(o))
	for k := range o {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (o Object) Get(key string) (Value, bool) {
	v, ok := o[key]
	return v, ok
}

// TryGet coerces a raw key u into the structure's key domain via the passed
// coercer, then looks it up. This is the idiomatic Go stand-in for a
// "U: Into<K>" bound — Go has no Into trait, so the coercion is a value.
//
//	v, ok, err := data.TryGet[int](obj, "0", strconv.Atoi)
//
// ok=false with err=nil means simply "not found"; a non-nil err means the raw
// key could not be coerced into K (e.g. "notanumber" for an int key).
func TryGet[K comparable, U any](d Discrete, u U, coerce func(U) (K, error)) (Value, bool, error) {
	k, err := coerce(u)
	if err != nil {
		var zero Value
		return zero, false, err
	}
	v, ok := d.Get(anyKeyString(k))
	return v, ok, nil
}

// anyKeyString renders a coerced key back to the string domain that Object is
// keyed on. For the common string-keyed case this is identity.
func anyKeyString[K comparable](k K) string {
	if s, ok := any(k).(string); ok {
		return s
	}
	return sprint(k)
}

// Transpose remaps a discrete value: for each src→dst pair it walks the src
// Path and places the result at dst in a fresh Object. It returns the new
// Object and the src paths that were missing (so callers can decide whether a
// miss is fatal). Backs the pick-map transformer.
func Transpose(v Value, srcs []Path, dsts []string) (Object, []Path) {
	out := make(Object, len(dsts))
	var missing []Path
	for i, src := range srcs {
		found, ok, err := Walk(v, src)
		if err != nil || !ok {
			missing = append(missing, src)
			continue
		}
		if i < len(dsts) {
			out[dsts[i]] = found
		}
	}
	return out, missing
}

// Map is the optional statically-typed escape hatch for homogeneous containers
// where V is a real concrete type (a CSV row with int keys, a uniform map).
// The non-generic Discrete above stays the mainline; parameterization only
// pays off here.
type Map[K comparable, V any] interface {
	Get(k K) (V, bool)
	TryGet(s string) (V, bool, error)
	Transpose(want, alt []K) (map[K]V, []K)
}
