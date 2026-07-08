package sdk

import "context"

// Parser decodes a config block into dst. Its shape matches
// ConfigBlock.Decode so that a bound block.Decode value may be handed
// directly to a Provider.
type Parser func(dst any) error

// Provider is a plugin author's factory: given a Parser it returns a
// configured Producer, Consumer, or Transformer.
type Provider[T Producer | Consumer | Transformer] func(parse Parser) (T, error)

// Producer emits data onto send. It reports errors on errs. It is
// responsible for closing send when done producing.
//
// Producer receives a context and must respect its cancellation by selecting
// on ctx.Done() alongside channel sends to avoid goroutine leaks when the host
// abandons the producer mid-run.
//
// Example:
//
//	func(ctx context.Context, send chan<- []byte, errs chan<- error) {
//	    defer close(send)
//	    for {
//	        data, err := getData()
//	        if err != nil {
//	            errs <- err
//	            return
//	        }
//	        select {
//	        case send <- data:
//	        case <-ctx.Done():
//	            errs <- ctx.Err()
//	            return
//	        }
//	    }
//	}
type Producer func(ctx context.Context, send chan<- []byte, errs chan<- error)

// Consumer receives data from recv. It reports errors on errs and
// signals completion by sending on done.
//
// Consumer receives a context and must respect its cancellation by selecting
// on ctx.Done() to avoid goroutine leaks when the host abandons the consumer
// mid-run.
//
// Example:
//
//	func(ctx context.Context, recv <-chan []byte, errs chan<- error, done chan<- struct{}) {
//	    for {
//	        select {
//	        case data, ok := <-recv:
//	            if !ok {
//	                done <- struct{}{}
//	                return
//	            }
//	            if err := processData(data); err != nil {
//	                errs <- err
//	                return
//	            }
//	        case <-ctx.Done():
//	            errs <- ctx.Err()
//	            return
//	        }
//	    }
//	}
type Consumer func(ctx context.Context, recv <-chan []byte, errs chan<- error, done chan<- struct{})

// Transformer maps one input datum to one output datum.
type Transformer func(in []byte) ([]byte, error)
