package fuzz

import (
	"bytes"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/http_utils"
	"github.com/pyneda/sukyan/pkg/payloads"
	"net/http"
	"sync"

	"github.com/rs/zerolog/log"
)

// HttpFuzzer is a reimplementation of the initial ParameterFuzzer which:
// - Should be easy configurable (consider callbacks)
// - Should allow all HTTP Methods
// - Allow all (or most) injection points ()
// - It's main use is for tests which do not require a browser
// - Concurrency (consider per host rate limit)
// - Use proxy
type HttpFuzzer struct {
	Concurrency int
	client      *http.Client
}

type HttpFuzzerTask struct {
	history        *db.History
	injectionPoint *InjectionPoint
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
func (f *HttpFuzzer) Run(history *db.History, payloads []payloads.PayloadInterface, injectionPoints []*InjectionPoint, results chan FuzzResult) {
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
		// Should review if the old insertion points it's worth to reuse or a need one needs to be implemented and handle the insertion points here.
		for _, injectionPoint := range injectionPoints {
			for _, payload := range payloads {
				task := HttpFuzzerTask{
					history:        history,
					payload:        payload,
					injectionPoint: injectionPoint,
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
		reqBody := bytes.NewReader(task.history.RequestBody)
		req, _ := http.NewRequest(task.history.Method, task.history.URL, reqBody)
		response, err := f.client.Do(req)
		// Could probably already create the history here and pass it to the result

		result.URL = task.history.URL // This should either be removed or be the URL with the payload
		result.Err = err
		result.Payload = task.payload
		result.Response = *response
		results <- result
		totalPendingChannel <- -1
	}
	wg.Done()
}
