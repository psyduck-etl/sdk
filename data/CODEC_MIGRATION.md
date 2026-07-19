# Codec Configuration Pattern for Plugins

This guide explains how to adopt the unified codec configuration pattern for your plugin's encode/decode operations.

## Overview

The `InputCodec` and `OutputCodec` types provide reusable, embeddable config fragments that handle:
- Lazy codec binding via `.Bind()` after parsing
- Validation of codec specs
- Encode/decode method delegation
- Terminal reference detection via `.Sparse()`
- Field metadata for resource specs via `.Spec()`

This eliminates 30-40 lines of duplicated codec boilerplate per plugin.

## Pattern: Embed Type Aliases

The recommended approach is to create type aliases in your plugin's `helpers.go`:

```go
package main

import (
	"github.com/psyduck-etl/sdk/data"
)

// acceptConfig is a backward-compatibility alias for data.InputCodec.
// See data.InputCodec for the canonical documentation and usage examples.
type acceptConfig = data.InputCodec

// emitConfig is a backward-compatibility alias for data.OutputCodec.
// See data.OutputCodec for the canonical documentation and usage examples.
type emitConfig = data.OutputCodec
```

This keeps your existing code unchanged while pointing to the SDK types.

## Using in Your Config

Embed the type aliases in your resource config structs:

```go
type MyConsumerConfig struct {
	Table string `psy:"table"`
	acceptConfig  // Embeds Accept field + .Bind(), .Decode(), .Sparse(), .Spec() methods
}

type MyTransformerConfig struct {
	Field string `psy:"field"`
	acceptConfig  // Input codec
	emitConfig    // Output codec
}
```

## In Your Provider

After parsing, call `.Bind()` to resolve the codec:

```go
func ProvideConsumer(ctx context.Context, parse sdk.Parser) (sdk.Consumer, error) {
	config := new(MyConsumerConfig)
	if err := parse(config); err != nil {
		return nil, err
	}
	
	// Bind resolves the codec via sdk.GetCodec()
	if err := config.acceptConfig.Bind(); err != nil {
		return nil, err
	}
	
	// Now config.Decode(bytes) is ready to use
	return &consumer{config: config}, nil
}
```

## Decoding Records

After `.Bind()`, use `.Decode()` to unmarshal records:

```go
func (c *consumer) consume(record []byte) error {
	// Decode returns any native type the codec produces
	decoded, err := c.config.Decode(record)
	if err != nil {
		return err
	}
	
	// Validate the shape your plugin needs
	if m, ok := decoded.(map[string]any); ok {
		// Handle structured object (json, yaml, csv, etc.)
		return c.handleMap(m)
	} else if s, ok := decoded.(string); ok {
		// Handle terminal reference (string codec)
		return c.handleID(s)
	}
	return fmt.Errorf("unexpected decoded type: %T", decoded)
}
```

## Detecting Terminal References

The `.Sparse()` method (or `sdk.data.IsTerminalRef()` helper) detects bare scalar references:

```go
// Branch behavior based on codec type
if config.acceptConfig.Sparse() {
	// Input is bare terminal references (IDs, names, etc.)
	// Fetch full objects from an API or database
	obj := api.Get(id)
} else {
	// Input is structured objects
	// Use them directly
	obj := decoded.(map[string]any)
}
```

Or use the standalone helper for one-off checks:

```go
if data.IsTerminalRef(someSpec) {
	// Handle terminal refs
}
```

## Field Metadata

Use `.Spec()` to include codec field definitions in your resource specs:

```go
var myResourceSpec = []*sdk.Spec{
	{
		Name:        "field",
		Description: "Your custom field",
		Required:    true,
		Type:        sdk.TypeString,
	},
	// Append the reusable codec spec
	myConfig.acceptConfig.Spec()...,
}
```

This adds the standard `accept` field definition to your resource spec.

## Handling Decoded Values

Different plugins handle decoded values differently — the SDK is shape-agnostic:

### SQL Plugin (mysql)
Expects `map[string]any` for SQL operations:
```go
func handleSQL(decoded any) error {
	m, ok := decoded.(map[string]any)
	if !ok {
		return fmt.Errorf("want object for SQL, got %T", decoded)
	}
	// Map to columns and execute INSERT/UPDATE
}
```

### API Plugin (ifunny)
Branches on terminal refs vs structured objects:
```go
func handleAPI(decoded any, isSparse bool) error {
	if isSparse {
		// decoded is string (ID) — fetch from API
		id := decoded.(string)
		obj := api.GetContent(id)
	} else {
		// decoded is map (structured object) — use directly
		obj := decoded.(map[string]any)
	}
	// Process obj
}
```

Define your own shape validation — the codec only handles binding and delegation.

## Reference Implementations

See these plugins for working examples:

- **mysql** (`github.com/psyduck-etl/mysql`):
  - Uses `acceptConfig` in consumer config
  - Uses `emitConfig` in producer config
  - Validates shapes with `decodeFor()` helper
  - Commit: 4a39a00 (migration to SDK types)

- **ifunny** (`github.com/psyduck-etl/ifunny`):
  - Uses both `acceptConfig` and `emitConfig` in transformer configs
  - Branches on `.Sparse()` to detect terminal vs structured input
  - Commit: 166f912 (migration to SDK types)

## Common Patterns

### Consumer that only decodes
```go
type ConsumerConfig struct {
	Table string `psy:"table"`
	acceptConfig  // Only input
}
```

### Producer that only encodes
```go
type ProducerConfig struct {
	Query string `psy:"query"`
	emitConfig  // Only output
```

### Transformer that does both
```go
type TransformerConfig struct {
	Field string `psy:"field"`
	acceptConfig  // Input
	emitConfig    // Output
}
```

### Bidirectional with composite
```go
type BidirectionalConfig struct {
	Field string `psy:"field"`
	data.CodecConfig  // Includes both acceptConfig + emitConfig
}

// In provider:
if err := config.CodecConfig.Bind(); err != nil {  // Binds both
	return nil, err
}
```

## Migration Checklist

If you're migrating an existing plugin:

- [ ] Add import: `github.com/psyduck-etl/sdk/data`
- [ ] Create type aliases in `helpers.go`
- [ ] Replace struct definitions with type aliases
- [ ] Update `.bind()` calls to `.Bind()` (uppercase per SDK convention)
- [ ] Remove any local `stringy()` helper — use `data.IsTerminalRef()` instead
- [ ] Run tests to verify behavior is unchanged
- [ ] Commit as refactoring (zero logic changes)

## Questions?

Refer to `sdk/data/codec_config.go` for the full API documentation and examples in the type docstrings.
