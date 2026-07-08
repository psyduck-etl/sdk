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
