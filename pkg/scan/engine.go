package scan

import (
	"github.com/pyneda/sukyan/db"
	"github.com/pyneda/sukyan/lib/integrations"
	"github.com/pyneda/sukyan/pkg/crawl"
	"github.com/pyneda/sukyan/pkg/passive"
	"github.com/rs/zerolog/log"

	"sync"
)

type ScanJobType string

const (
	ScanJobTypePassive ScanJobType = "passive"
	ScanJobTypeActive  ScanJobType = "active"
	ScanJobTypeAll     ScanJobType = "all"
)

type ScanEngine struct {
	task                      *db.Task
	MaxConcurrentPassiveScans int
	MaxConcurrentActiveScans  int
	InteractionsManager       *integrations.InteractionsManager
	passiveScanCh             chan *db.History
	activeScanCh              chan *db.History
	wg                        sync.WaitGroup
	stopCh                    chan struct{}
	pauseCh                   chan struct{}
	resumeCh                  chan struct{}
	isPaused                  bool
}

func NewScanEngine(maxConcurrentPassiveScans, maxConcurrentActiveScans int, interactionsManager *integrations.InteractionsManager) *ScanEngine {
	return &ScanEngine{
		MaxConcurrentPassiveScans: maxConcurrentPassiveScans,
		MaxConcurrentActiveScans:  maxConcurrentActiveScans,
		InteractionsManager:       interactionsManager,
		passiveScanCh:             make(chan *db.History, maxConcurrentPassiveScans),
		activeScanCh:              make(chan *db.History, maxConcurrentActiveScans),
		stopCh:                    make(chan struct{}),
		pauseCh:                   make(chan struct{}),
		resumeCh:                  make(chan struct{}),
	}
}

// func (s *ScanEngine) ScheduleHistoryItemScan(item *db.History, scanJobType ScanJobType) {
//     s.wg.Add(1)
//     switch scanJobType {
//     case ScanJobTypePassive:
//         go func(item *db.History) {
//             defer s.wg.Done()
//             s.schedulePassiveScan(item)
//         }(item)
//     case ScanJobTypeActive:
//         go func(item *db.History) {
//             defer s.wg.Done()
//             s.scheduleActiveScan(item)
//         }(item)
//     case ScanJobTypeAll:
//         go func(item *db.History) {
//             defer s.wg.Done()
//             s.schedulePassiveScan(item)
//         }(item)
//         go func(item *db.History) {
//             defer s.wg.Done()
//             s.scheduleActiveScan(item)
//         }(item)
//     }
// }

func (s *ScanEngine) ScheduleHistoryItemScan(item *db.History, scanJobType ScanJobType) {
	switch scanJobType {
	case ScanJobTypePassive:
		s.passiveScanCh <- item
	case ScanJobTypeActive:
		s.activeScanCh <- item
	case ScanJobTypeAll:
		s.passiveScanCh <- item
		s.activeScanCh <- item
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
				s.schedulePassiveScan(item)
			}(item)
		case item := <-s.activeScanCh:
			s.wg.Add(1)
			go func(item *db.History) {
				defer s.wg.Done()
				s.scheduleActiveScan(item)
			}(item)
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

func (s *ScanEngine) schedulePassiveScan(item *db.History) {
	passive.ScanHistoryItem(item)
}

func (s *ScanEngine) scheduleActiveScan(item *db.History) {
	ActiveScanHistoryItem(item, s.InteractionsManager)
}

func (s *ScanEngine) CrawlAndAudit(startUrls []string, maxPagesToCrawl, depth, pagesPoolSize int, waitCompletion bool) {
	crawler := crawl.NewCrawler(startUrls, maxPagesToCrawl, depth, pagesPoolSize)
	historyItems := crawler.Run()
	log.Info().Int("count", len(historyItems)).Msg("Crawling finished, scheduling active scans")
	for _, historyItem := range historyItems {
		if historyItem.StatusCode == 404 {
			continue
		}
		s.ScheduleHistoryItemScan(historyItem, ScanJobTypeActive)
	}
	log.Info().Msg("Active scans scheduled")
	if waitCompletion {
		s.wg.Wait()
		log.Info().Msg("Active scans finished")
	}
}
