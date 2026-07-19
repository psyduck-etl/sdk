package sdk

import "context"

// Parser decodes a config block into dst. Its shape matches
// ConfigBlock.Decode so that a bound block.Decode value may be handed
// directly to a Provider.
type Parser func(dst any) error

// Provider is a plugin author's factory: given a context and a Parser it
// returns a configured Producer, Consumer, or Transformer. The context allows
// providers to perform cancellable setup work (e.g. database schema bootstrap)
// at bind time. Cancelling ctx will cause the provider to abort setup and
// return an error.
type Provider[T Producer | Consumer | Transformer] func(ctx context.Context, parse Parser) (T, error)

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

// Transformer reads data from in and writes results to out. It reports
// errors on errs. It is responsible for closing out when done — typically
// when in is closed and any buffered state has been flushed.
//
// Transformer receives a context and must respect its cancellation by
// selecting on ctx.Done() alongside channel sends to avoid goroutine leaks
// when the host abandons the transformer mid-run. A Transformer must not
// close errs.
//
// To filter a message out, simply do not write it to out. A stateful
// Transformer (e.g. batching or windowing) should flush any buffered output
// once in is closed, before returning.
//
// Example:
//
//	func(ctx context.Context, in <-chan []byte, out chan<- []byte, errs chan<- error) {
//	    defer close(out)
//	    for data := range in {
//	        transformed, err := transformData(data)
//	        if err != nil {
//	            errs <- err
//	            continue
//	        }
//	        select {
//	        case out <- transformed:
//	        case <-ctx.Done():
//	            return
//	        }
//	    }
//	}
type Transformer func(ctx context.Context, in <-chan []byte, out chan<- []byte, errs chan<- error)
