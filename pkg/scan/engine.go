package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib"
	"github.com/pyneda/sukyan/lib/integrations"

	"github.com/pyneda/sukyan/pkg/crawl"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/pyneda/sukyan/pkg/payloads/generation"
	"github.com/rs/zerolog/log"
	"github.com/spf13/viper"
	"sync"
	"time"
)

type ScanJobType string

const (
	ScanJobTypePassive ScanJobType = "passive"
	ScanJobTypeActive  ScanJobType = "active"
	ScanJobTypeAll     ScanJobType = "all"
)

type engineTask struct {
	item        *db.History
	workspaceID uint
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

func (s *ScanEngine) ScheduleHistoryItemScan(item *db.History, scanJobType ScanJobType, workspaceID uint) {
	switch scanJobType {
	case ScanJobTypePassive:
		s.passiveScanCh <- item
	case ScanJobTypeActive:
		s.activeScanCh <- &engineTask{item: item, workspaceID: workspaceID}
	case ScanJobTypeAll:
		s.passiveScanCh <- item
		s.activeScanCh <- &engineTask{item: item, workspaceID: workspaceID}
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
				s.scheduleActiveScan(task.item, task.workspaceID)
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

func (s *ScanEngine) scheduleActiveScan(item *db.History, workspaceID uint) {
	ActiveScanHistoryItem(item, s.InteractionsManager, s.payloadGenerators, workspaceID)
}

func (s *ScanEngine) CrawlAndAudit(startUrls []string, maxPagesToCrawl, depth, pagesPoolSize int, waitCompletion bool, excludePatterns []string, workspaceID uint) {
	task, err := db.Connection.NewTask(workspaceID, "crawl")
	if err != nil {
		log.Error().Err(err).Msg("Could not create task")
	}
	crawler := crawl.NewCrawler(startUrls, maxPagesToCrawl, depth, pagesPoolSize, excludePatterns, workspaceID)
	historyItems := crawler.Run()
	uniqueHistoryItems := removeDuplicateHistoryItems(historyItems)
	log.Info().Int("count", len(uniqueHistoryItems)).Msg("Crawling finished, scheduling active scans")
	fingerprints := passive.FingerprintHistoryItems(uniqueHistoryItems)
	log.Info().Int("count", len(fingerprints)).Interface("fingerprints", fingerprints).Msg("Gathered fingerprints")

	baseURLs, err := lib.GetUniqueBaseURLs(startUrls)
	if err != nil {
		log.Error().Err(err).Msg("Could not get unique base urls")
	}

	// Very basic initial integration, could probably launch it in parallel with other tasks
	if viper.GetBool("integrations.nuclei.enabled") {
		db.Connection.SetTaskStatus(task.ID, db.TaskStatusNuclei)
		nucleiTags := passive.GetUniqueNucleiTags(fingerprints)
		log.Info().Int("count", len(nucleiTags)).Interface("tags", nucleiTags).Msg("Gathered tags from fingerprints for Nuclei scan")
		nucleiScanErr := integrations.NucleiScan(baseURLs, workspaceID)
		if nucleiScanErr != nil {
			log.Error().Err(nucleiScanErr).Msg("Error running nuclei scan")
		}
	}

	retireScanner := integrations.NewRetireScanner()
	db.Connection.SetTaskStatus(task.ID, db.TaskStatusScanning)

	for _, historyItem := range uniqueHistoryItems {
		if historyItem.StatusCode == 404 {
			continue
		}
		s.ScheduleHistoryItemScan(historyItem, ScanJobTypeAll, workspaceID)
		retireScanner.HistoryScan(historyItem)
	}
	log.Info().Msg("Active scans scheduled")
	if waitCompletion {
		time.Sleep(3 * time.Second)
		s.wg.Wait()
		log.Info().Msg("Active scans finished")
	}
	db.Connection.SetTaskStatus(task.ID, db.TaskStatusFinished)

}
