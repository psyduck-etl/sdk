package sdk

// ProduceFrom repeatedly invokes next and forwards its output to send. It
// returns when next signals done, or forwards next's error verbatim.
func ProduceFrom(next func() ([]byte, bool, error), send chan<- []byte) error {
	for {
		dataNext, done, err := next()
		if err != nil {
			return err
		} else if done {
			return nil
		}

		send <- dataNext
	}
}

// ConsumeInto reads from recv until it closes, invoking next on each
// item. Rate limiting is a host concern and is no longer performed here.
func ConsumeInto(next func([]byte) error, recv <-chan []byte) error {
	for dataNext := range recv {
		if err := next(dataNext); err != nil {
			return err
		}
	}

	return nil
}
