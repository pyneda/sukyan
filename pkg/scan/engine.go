package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"

	"strings"
	"sync"
	"time"

	"github.com/pyneda/sukyan/pkg/crawl"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
)

type ScanJobType string

const (
	ScanJobTypePassive ScanJobType = "passive"
	ScanJobTypeActive  ScanJobType = "active"
	ScanJobTypeAll     ScanJobType = "all"
)

type engineTask struct {
	item    *db.History
	taskJob *db.TaskJob
	options HistoryItemScanOptions
}

type ScanEngine struct {
	task                      *db.Task
	MaxConcurrentPassiveScans int
	MaxConcurrentActiveScans  int
	InteractionsManager       *integrations.InteractionsManager
	payloadGenerators         []*generation.PayloadGenerator
	passiveScanCh             chan *db.History
	activeScanCh              chan *engineTask
	wg                        sync.WaitGroup
	stopCh                    chan struct{}
	pauseCh                   chan struct{}
	resumeCh                  chan struct{}
	isPaused                  bool
}

func NewScanEngine(payloadGenerators []*generation.PayloadGenerator, maxConcurrentPassiveScans, maxConcurrentActiveScans int, interactionsManager *integrations.InteractionsManager) *ScanEngine {
	return &ScanEngine{
		MaxConcurrentPassiveScans: maxConcurrentPassiveScans,
		MaxConcurrentActiveScans:  maxConcurrentActiveScans,
		InteractionsManager:       interactionsManager,
		passiveScanCh:             make(chan *db.History, maxConcurrentPassiveScans),
		activeScanCh:              make(chan *engineTask, maxConcurrentActiveScans),
		stopCh:                    make(chan struct{}),
		pauseCh:                   make(chan struct{}),
		resumeCh:                  make(chan struct{}),
		payloadGenerators:         payloadGenerators,
	}
}

func (s *ScanEngine) ScheduleHistoryItemScan(item *db.History, scanJobType ScanJobType, options HistoryItemScanOptions) {

	switch scanJobType {
	case ScanJobTypePassive:
		s.passiveScanCh <- item
	case ScanJobTypeActive:
		taskJob, err := db.Connection.NewTaskJob(options.TaskID, "Active scan", db.TaskJobScheduled, item.ID)
		if err != nil {
			log.Error().Err(err).Interface("type", scanJobType).Uint("history", item.ID).Msg("Could not create task job")
			return
		}
		s.activeScanCh <- &engineTask{item: item, taskJob: taskJob, options: options}
	case ScanJobTypeAll:
		taskJob, err := db.Connection.NewTaskJob(options.TaskID, "Active and passive scan", db.TaskJobScheduled, item.ID)
		if err != nil {
			log.Error().Err(err).Interface("type", scanJobType).Uint("history", item.ID).Msg("Could not create task job")
			return
		}
		s.passiveScanCh <- item
		s.activeScanCh <- &engineTask{item: item, taskJob: taskJob, options: options}
	}
}

func (s *ScanEngine) Start() {
	go s.run()
}

func (s *ScanEngine) Stop() {
	close(s.stopCh)
	s.wg.Wait()
}

func (s *ScanEngine) Pause() {
	s.pauseCh <- struct{}{}
}

func (s *ScanEngine) Resume() {
	s.resumeCh <- struct{}{}
}

func (s *ScanEngine) run() {
	for {
		select {
		case item := <-s.passiveScanCh:
			s.wg.Add(1)
			go func(item *db.History) {
				defer s.wg.Done()
				s.schedulePassiveScan(item, 0)
			}(item)
		case task := <-s.activeScanCh:
			s.wg.Add(1)
			go func(task *engineTask) {
				defer s.wg.Done()
				s.scheduleActiveScan(task.item, task.taskJob, task.options)
			}(task)
		case <-s.pauseCh:
			s.isPaused = true
		case <-s.resumeCh:
			s.isPaused = false
		case <-s.stopCh:
			close(s.passiveScanCh)
			close(s.activeScanCh)
			return
		}

		if s.isPaused {
			<-s.resumeCh
		}
	}
}

func (s *ScanEngine) schedulePassiveScan(item *db.History, workspaceID uint) {
	passive.ScanHistoryItem(item)
}

func (s *ScanEngine) scheduleActiveScan(item *db.History, taskJob *db.TaskJob, options HistoryItemScanOptions) {
	options.TaskJobID = taskJob.ID
	taskJob.Status = db.TaskJobRunning
	db.Connection.UpdateTaskJob(taskJob)
	ActiveScanHistoryItem(item, s.InteractionsManager, s.payloadGenerators, options)
	taskJob.Status = db.TaskJobFinished
	taskJob.CompletedAt = time.Now()
	db.Connection.UpdateTaskJob(taskJob)
}

func (s *ScanEngine) FullScan(options FullScanOptions, waitCompletion bool) {
	task, err := db.Connection.NewTask(options.WorkspaceID, nil, options.Title, db.TaskStatusCrawling)
	if err != nil {
		log.Error().Err(err).Msg("Could not create task")
	}
	ignoredExtensions := viper.GetStringSlice("crawl.ignored_extensions")

	scanLog := log.With().Uint("task", task.ID).Str("title", options.Title).Uint("workspace", options.WorkspaceID).Logger()
	crawler := crawl.NewCrawler(options.StartURLs, options.MaxPagesToCrawl, options.MaxDepth, options.PagesPoolSize, options.ExcludePatterns, options.WorkspaceID, task.ID, options.Headers)
	historyItems := crawler.Run()
	if len(historyItems) == 0 {
		db.Connection.SetTaskStatus(task.ID, db.TaskStatusFinished)
		scanLog.Info().Msg("No history items gathered during crawl, exiting")
		return
	}
	uniqueHistoryItems := removeDuplicateHistoryItems(historyItems)
	scanLog.Info().Int("count", len(uniqueHistoryItems)).Msg("Crawling finished, scheduling active scans")
	fingerprints := make([]passive.Fingerprint, 0)
	scanLog.Info().Int("count", len(fingerprints)).Interface("fingerprints", fingerprints).Msg("Gathered fingerprints")

	historiesByBaseURL := separateHistoriesByBaseURL(uniqueHistoryItems)
	for baseURL, histories := range historiesByBaseURL {
		passive.AnalyzeHeaders(baseURL, histories)
		newFingerprints := passive.FingerprintHistoryItems(histories)
		passive.ReportFingerprints(baseURL, newFingerprints, options.WorkspaceID, task.ID)
		fingerprints = append(fingerprints, newFingerprints...)
		integrations.CDNCheck(baseURL, options.WorkspaceID, task.ID)
	}

	baseURLs, err := lib.GetUniqueBaseURLs(options.StartURLs)
	if err != nil {
		log.Error().Err(err).Msg("Could not get unique base urls")
	}

	fingerprintTags := passive.GetUniqueNucleiTags(fingerprints)

	if viper.GetBool("integrations.nuclei.enabled") {
		db.Connection.SetTaskStatus(task.ID, db.TaskStatusNuclei)
		scanLog.Info().Int("count", len(fingerprintTags)).Interface("tags", fingerprintTags).Msg("Gathered tags from fingerprints for Nuclei scan")
		nucleiScanErr := integrations.NucleiScan(baseURLs, options.WorkspaceID)
		if nucleiScanErr != nil {
			scanLog.Error().Err(nucleiScanErr).Msg("Error running nuclei scan")
		}
	}

	retireScanner := integrations.NewRetireScanner()

	db.Connection.SetTaskStatus(task.ID, db.TaskStatusScanning)
	itemScanOptions := HistoryItemScanOptions{
		WorkspaceID:     options.WorkspaceID,
		TaskID:          task.ID,
		Mode:            options.Mode,
		InsertionPoints: options.InsertionPoints,
		FingerprintTags: fingerprintTags,
	}

	for _, historyItem := range uniqueHistoryItems {
		if historyItem.StatusCode == 404 {
			continue
		}
		go retireScanner.HistoryScan(historyItem)

		shouldSkip := false

		for _, extension := range ignoredExtensions {
			if strings.HasSuffix(historyItem.URL, extension) {
				shouldSkip = true
				break
			}
		}

		if shouldSkip {
			continue
		}

		s.ScheduleHistoryItemScan(historyItem, ScanJobTypeAll, itemScanOptions)

	}
	scanLog.Info().Msg("Active scans scheduled")
	if waitCompletion {
		time.Sleep(3 * time.Second)
		s.wg.Wait()
		scanLog.Info().Msg("Active scans finished")
		db.Connection.SetTaskStatus(task.ID, db.TaskStatusFinished)

	} else {
		go func() {
			s.wg.Wait()
			scanLog.Info().Msg("Active scans finished")
			db.Connection.SetTaskStatus(task.ID, db.TaskStatusFinished)
		}()
	}
}
