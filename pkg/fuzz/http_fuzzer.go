package fuzz

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

type HistoryFuzzResult struct {
	Original       *db.History
	Response       http.Response
	Err            error
	Payload        *generation.Payload
	InsertionPoint InsertionPoint
}

type HttpFuzzer struct {
	Concurrency         int
	InteractionsManager *integrations.InteractionsManager
	client              *http.Client
}

type HttpFuzzerTask struct {
	history        *db.History
	insertionPoint InsertionPoint
	payload        *generation.Payload
}

func (f *HttpFuzzer) checkConfig() {
	if f.Concurrency == 0 {
		log.Info().Interface("fuzzer", f).Msg("Concurrency is not set, setting 4 as default")
		f.Concurrency = 4
	}
	if f.client == nil {
		f.client = http_utils.CreateHttpClient()
	}
}

// Run starts the fuzzing job
func (f *HttpFuzzer) Run(history *db.History, payloadGenerators []*generation.PayloadGenerator, insertionPoints []InsertionPoint) {

	var wg sync.WaitGroup
	f.checkConfig()
	// Declare the channels
	pendingTasks := make(chan HttpFuzzerTask, f.Concurrency)
	defer close(pendingTasks)

	// Schedule workers
	for i := 0; i < f.Concurrency; i++ {
		go f.worker(&wg, pendingTasks)
	}

	for _, insertionPoint := range insertionPoints {
		log.Debug().Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Msgf("Scanning insertion point: %s", insertionPoint)
		for _, generator := range payloadGenerators {
			payloads, err := generator.BuildPayloads(*f.InteractionsManager)
			if err != nil {
				log.Error().Err(err).Msg("Failed to build payloads")
				continue
			}
			for _, payload := range payloads {
				wg.Add(1)
				task := HttpFuzzerTask{
					history:        history,
					payload:        &payload,
					insertionPoint: insertionPoint,
				}
				pendingTasks <- task
			}
		}
	}
	log.Debug().Msg("Waiting for all the fuzzing tasks to finish")
	wg.Wait()
	log.Debug().Str("item", history.URL).Str("method", history.Method).Int("ID", int(history.ID)).Msg("Finished fuzzing history item")
}

// worker makes the request and processes the result
func (f *HttpFuzzer) worker(wg *sync.WaitGroup, pendingTasks chan HttpFuzzerTask) {
	for task := range pendingTasks {
		log.Debug().Interface("task", task).Msg("New fuzzer task received by parameter worker")
		var result HistoryFuzzResult
		builders := []InsertionPointBuilder{
			InsertionPointBuilder{
				Point:   task.insertionPoint,
				Payload: task.payload.Value,
			},
		}

		req, err := CreateRequestFromInsertionPoints(task.history, builders)
		if err != nil {
			log.Error().Err(err).Str("method", task.history.Method).Str("param", task.insertionPoint.Name).Str("payload", task.payload.Value).Str("url", task.history.URL).Msg("Error building request from insertion points")
			result.Err = err
		} else {
			response, err := f.client.Do(req)
			result.Err = err
			result.Response = *response
			// TODO: Here instead of creating a HistoryFuzzResult, should evaluate the response agains the detection methods
		}
		result.Payload = task.payload
		result.InsertionPoint = task.insertionPoint
		result.Original = task.history
		wg.Done()
	}
}
