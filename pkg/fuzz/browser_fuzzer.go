package fuzz

import (
	"sync"
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/pkg/payloads"

	"github.com/rs/zerolog/log"
)

type BrowserFuzzTask struct {
	InjectionPoint  *URLInjectionPoint
	Payload         *payloads.PayloadInterface
	PreloadCallback func()
	LoadedCallback  func()
}

type BrowserFuzzTaskResult struct {
	Task   BrowserFuzzTask
	Issues []db.Issue
	//BrowserEvents []events.BrowserEvent
}

// BrowserFuzzer differs from the previous one in:
// - Test cases are BrowserFuzzTasks that can be added after the fuzzer has started based on previous results
type BrowserFuzzer struct {
	Config             FuzzerConfig
	URLInjectionPoints []URLInjectionPoint
}

func (f *BrowserFuzzer) checkConfig() {
	if f.Config.Concurrency == 0 {
		log.Info().Interface("fuzzer", f).Msg("Concurrency is not set, setting 4 as default")
		f.Config.Concurrency = 4
	}
}

func (f *BrowserFuzzer) Run(tasksChannel chan BrowserFuzzTask) {

}

func (f *BrowserFuzzer) Worker(wg *sync.WaitGroup, tasksChannel chan BrowserFuzzTask) {
	for task := range tasksChannel {
		log.Debug().Interface("task", task).Msg("Browser fuzz received new task")
		// result := BrowserFuzzTaskResult{
		// 	Task: task,
		// }
		//task.Callback()
		// results <- result
	}
	wg.Done()
}
