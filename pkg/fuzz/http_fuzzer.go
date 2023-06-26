package fuzz

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

// HttpFuzzer is a reimplementation of the initial ParameterFuzzer which:
// - Is based on a Http history and supports all HTTP methods
// - Supports multiple insertion points (URL, Header, Body, Cookie)
type HttpFuzzer struct {
	Concurrency int
	client      *http.Client
}

type HttpFuzzerTask struct {
	history        *db.History
	insertionPoint *InsertionPoint
	payload        payloads.PayloadInterface
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
func (f *HttpFuzzer) Run(history *db.History, payloads []payloads.PayloadInterface, insertionPoints []*InsertionPoint, results chan FuzzResult) {
	var wg sync.WaitGroup
	// TODO: The concurrency implementation should be reviewed, the WaitGroup is not being waited
	f.checkConfig()
	// Declare the channels
	totalPendingChannel := make(chan int)
	pendingTasks := make(chan HttpFuzzerTask)

	go f.monitor(pendingTasks, results, totalPendingChannel)
	// Schedule workers
	for i := 0; i < f.Concurrency; i++ {
		wg.Add(1)
		go f.worker(&wg, pendingTasks, results, totalPendingChannel)
	}

	go func() {
		// By now, just one insertion point is supported even though the CreateRequestFromInsertionPoints function supports multiple
		for _, insertionPoint := range insertionPoints {
			for _, payload := range payloads {
				task := HttpFuzzerTask{
					history:        history,
					payload:        payload,
					insertionPoint: insertionPoint,
				}
				pendingTasks <- task
				totalPendingChannel <- 1
			}
		}
	}()
}

// monitor checks when the job has finished
func (f *HttpFuzzer) monitor(pendingTasks chan HttpFuzzerTask, results chan FuzzResult, totalPendingChannel chan int) {
	count := 0
	for c := range totalPendingChannel {
		count += c
		if count == 0 {
			log.Debug().Msg("HttpFuzzer monitor closing all the communication channels")
			close(pendingTasks)
			close(totalPendingChannel)
			close(results)
		}
	}
}

// worker makes the request and processes the result
func (f *HttpFuzzer) worker(wg *sync.WaitGroup, pendingTasks chan HttpFuzzerTask, results chan FuzzResult, totalPendingChannel chan int) {
	for task := range pendingTasks {
		// make the request and store in result and then pass it fiz results channel
		log.Debug().Interface("task", task).Msg("New fuzzer task received by parameter worker")
		var result FuzzResult
		builders := []InsertionPointBuilder{
			InsertionPointBuilder{
				Point:   *task.insertionPoint,
				Payload: task.payload.GetValue(),
			},
		}
		req, err := CreateRequestFromInsertionPoints(task.history, builders)
		if err != nil {
			log.Error().Err(err).Str("param", task.insertionPoint.Name).Str("payload", task.payload.GetValue()).Str("url", task.history.URL).Msg("Error building request from insertion points")
			result.Err = err

		} else {
			response, err := f.client.Do(req)
			result.Err = err
			result.URL = response.Request.URL.String()
			result.Response = *response
		}
		result.Payload = task.payload

		results <- result
		totalPendingChannel <- -1
	}
	wg.Done()
}
