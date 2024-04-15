package sdk

import "time"

const EACH_MINUTE = 60_000

func ratelimit(perMinute uint) func() {
	if perMinute == 0 {
		return func() {}
	}

	d := time.Millisecond * time.Duration(EACH_MINUTE/perMinute)
	return func() { time.Sleep(d) }
}

// Produce data returned from successive calls to next
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

type ConsumeIntoConfig struct {
	PerMinute uint `psy:"per-minute"`
}

// Consume data streamed and call next on it
func ConsumeInto(next func([]byte) error, config ConsumeIntoConfig, recv <-chan []byte) error {
	wait := ratelimit(config.PerMinute)
	for dataNext := range recv {
		err := next(dataNext)
		if err != nil {
			return err
		}

		wait()
	}

	return nil
}
