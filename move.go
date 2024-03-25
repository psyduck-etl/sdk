package sdk

import "time"

const EACH_MINUTE = 60_000

type PipelineConfig struct {
	PerMinute   int  `psy:"per-minute"`
	ExitOnError bool `psy:"exit-on-error"`
}

func specPipelineConfig() SpecMap {
	return SpecMap{
		"per-minute":    SpecPerMinute(180),
		"exit-on-error": SpecExitOnError(true),
	}
}

func mustParse(parse SpecParser) *PipelineConfig {
	config := new(PipelineConfig)
	if err := parse(specPipelineConfig(), config); err != nil {
		panic(err)
	}

	return config
}

func ratelimit(perMinute int) {
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
	config := mustParse(parse)

	for dataNext := range recv {
		err := next(dataNext)
		if err != nil {
			return err
		}

		ratelimit(config.PerMinute)
	}

	return nil
}
