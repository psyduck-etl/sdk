package sdk

import "time"

const EACH_MINUTE = 60_000

func ratelimit(perMinute uint) {
	if perMinute == 0 {
		return
	}

	time.Sleep(time.Millisecond * time.Duration(EACH_MINUTE/perMinute))
}

// Produce data returned from successive calls to next
func ProduceChunk(next func() ([]byte, bool, error), parse SpecParser, send chan<- []byte) error {
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

// Consume data streamed and call next on it
func ConsumeChunk(next func([]byte) error, parse SpecParser, recv <-chan []byte) error {
	config := new(sdkConfig)
	if err := parse(PipelineSpec(), config); err != nil {
		return err
	}

	for dataNext := range recv {
		err := next(dataNext)
		if err != nil {
			return err
		}

		ratelimit(config.PerMinute)
	}

	return nil
}
