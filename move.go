package sdk

import "context"

// ProduceFrom repeatedly invokes next and forwards its output to send. It
// returns when next signals done, or forwards next's error verbatim.
// ProduceFrom respects ctx cancellation.
func ProduceFrom(ctx context.Context, next func() ([]byte, bool, error), send chan<- []byte) error {
	for {
		dataNext, done, err := next()
		if err != nil {
			return err
		} else if done {
			return nil
		}

		select {
		case send <- dataNext:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// MapContext lifts a one-to-one mapping function onto the Transformer stage
// contract: fn is applied to each input item and its output emitted
// downstream. The fn receives the stage ctx, so per-record work (e.g. network IO)
// is bound by the transformer's cancellation. Returning (nil, nil) emits nothing —
// the item is filtered out. An error is reported on errs and the item dropped;
// the stage keeps running. MapContext closes out on the way out and respects ctx
// cancellation.
func MapContext(fn func(context.Context, []byte) ([]byte, error)) Transformer {
	return func(ctx context.Context, in <-chan []byte, out chan<- []byte, errs chan<- error) {
		defer close(out)
		for {
			select {
			case data, ok := <-in:
				if !ok {
					return
				}
				mapped, err := fn(ctx, data)
				if err != nil {
					select {
					case errs <- err:
					case <-ctx.Done():
						return
					}
					continue
				}
				if mapped == nil {
					continue
				}
				select {
				case out <- mapped:
				case <-ctx.Done():
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}
}

// Map lifts a one-to-one mapping function onto the Transformer stage
// contract: fn is applied to each input item and its output emitted
// downstream. Returning (nil, nil) emits nothing — the item is filtered
// out. An error is reported on errs and the item dropped; the stage keeps
// running. Map closes out on the way out and respects ctx cancellation,
// so fn is all a simple stateless transformer needs to provide.
func Map(fn func([]byte) ([]byte, error)) Transformer {
	return MapContext(func(_ context.Context, d []byte) ([]byte, error) { return fn(d) })
}

// ConsumeInto reads from recv until it closes, invoking next on each
// item. Rate limiting is a host concern and is no longer performed here.
// ConsumeInto respects ctx cancellation.
func ConsumeInto(ctx context.Context, next func([]byte) error, recv <-chan []byte) error {
	for {
		select {
		case dataNext, ok := <-recv:
			if !ok {
				return nil
			}
			if err := next(dataNext); err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
