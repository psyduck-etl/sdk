// generate.go holds the canonical protobuf codegen invocation for this
// package: `go generate ./proto` (protoc + protoc-gen-go{,-grpc} on PATH —
// `nix develop` provides them). The pre-commit hook in .githooks runs this
// whenever psyduck.proto is part of a commit and refuses stale output.
package proto

//go:generate protoc --go_out=. --go_opt=paths=source_relative --go-grpc_out=. --go-grpc_opt=paths=source_relative psyduck.proto
