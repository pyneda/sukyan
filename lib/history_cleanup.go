package lib

import (
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/iter"
	"github.com/sourcegraph/conc/pool"
	"gorm.io/gorm"
)

type HistoryCleanupOptions struct {
	MaxBodySize       int      `json:"max_body_size" validate:"min=0"`
	WorkspaceID       *uint    `json:"workspace_id,omitempty" validate:"omitempty,min=1"`
	Sources           []string `json:"sources,omitempty" validate:"omitempty,dive,max=100"`
	IncludeWebSockets bool     `json:"include_websockets"`
	BatchSize         int      `json:"batch_size" validate:"min=1,max=10000"`
	DryRun            bool     `json:"dry_run"`
	Workers           int      `json:"workers" validate:"min=1,max=20"`
	MaxRecords        int64    `json:"max_records" validate:"min=0"`
}

type HistoryCleanupResult struct {
	ProcessedCount int64 `json:"processed_count"`
	TrimmedCount   int64 `json:"trimmed_count"`
	BytesSaved     int64 `json:"bytes_saved"`
	DryRun         bool  `json:"dry_run"`
}

type History struct {
	ID                 uint   `gorm:"primaryKey"`
	RawRequest         []byte `json:"raw_request"`
	RawResponse        []byte `json:"raw_response"`
	WorkspaceID        *uint  `json:"workspace_id" gorm:"index"`
	Source             string `gorm:"index" json:"source"`
	IsWebSocketUpgrade bool   `json:"is_websocket_upgrade"`
}

type BatchUpdateItem struct {
	ID          uint
	RawRequest  []byte
	RawResponse []byte
	BytesSaved  int64
}

func TrimHistoryBodies(db *gorm.DB, options HistoryCleanupOptions) (*HistoryCleanupResult, error) {
	if options.BatchSize <= 0 {
		options.BatchSize = 1000
	}
	if options.MaxBodySize < 0 {
		return nil, fmt.Errorf("max_body_size must be non-negative")
	}
	if options.Workers <= 0 {
		options.Workers = 4
	}

	result := &HistoryCleanupResult{
		DryRun: options.DryRun,
	}

	query := db.Model(&History{}).
		Where("(LENGTH(raw_request) > ? OR LENGTH(raw_response) > ?)", options.MaxBodySize, options.MaxBodySize).
		Select("id, raw_request, raw_response")

	if options.WorkspaceID != nil {
		query = query.Where("workspace_id = ?", *options.WorkspaceID)
	}

	if len(options.Sources) > 0 {
		query = query.Where("source IN ?", options.Sources)
	}

	if !options.IncludeWebSockets {
		query = query.Where("is_web_socket_upgrade = ?", false)
	}

	var totalCount int64
	if err := query.Count(&totalCount).Error; err != nil {
		log.Error().Err(err).Msg("Failed to count history records for cleanup")
		return nil, fmt.Errorf("failed to count records: %w", err)
	}

	log.Info().Int64("total_records", totalCount).Interface("options", options).Msg("Starting history body cleanup")

	if totalCount == 0 {
		log.Warn().Msg("No records found matching the criteria")
		if len(options.Sources) > 0 {
			var availableSources []string
			if err := db.Model(&History{}).Distinct("source").Limit(10).Pluck("source", &availableSources).Error; err == nil {
				log.Info().Interface("available_sources", availableSources).Msg("Available sources in database (first 10)")
			}
		}
		return result, nil
	}

	if options.DryRun {
		return performDryRunAnalysis(query, totalCount, options)
	}

	return performOptimizedCleanup(db, query, totalCount, options)
}

func performOptimizedCleanup(db *gorm.DB, query *gorm.DB, totalCount int64, options HistoryCleanupOptions) (*HistoryCleanupResult, error) {
	result := &HistoryCleanupResult{DryRun: false}

	fetchBatchSize := 2000
	processBatchSize := 100

	var lastID uint = 0
	var mu sync.Mutex

	// Track how many records we've trimmed if there's a limit
	var recordsTrimmeddSoFar int64 = 0

	for {
		start := time.Now()

		// If we have a MaxRecords limit and we've reached it, stop
		if options.MaxRecords > 0 && recordsTrimmeddSoFar >= options.MaxRecords {
			log.Info().
				Int64("records_trimmed", recordsTrimmeddSoFar).
				Int64("max_records", options.MaxRecords).
				Msg("Reached maximum records limit, stopping")
			break
		}

		var histories []History
		batchQuery := query.Where("id > ?", lastID).Order("id ASC").Limit(fetchBatchSize).Find(&histories)
		if batchQuery.Error != nil {
			log.Error().Err(batchQuery.Error).Uint("last_id", lastID).Msg("Failed to fetch history batch")
			return result, fmt.Errorf("failed to fetch batch after ID %d: %w", lastID, batchQuery.Error)
		}

		if len(histories) == 0 {
			break
		}

		fetchTime := time.Since(start)
		log.Info().
			Int("records_in_batch", len(histories)).
			Dur("fetch_time", fetchTime).
			Msg("Fetched batch")

		lastID = histories[len(histories)-1].ID

		chunks := chunkHistories(histories, processBatchSize)

		p := pool.NewWithResults[*BatchProcessResult]().WithMaxGoroutines(options.Workers)

		for _, chunk := range chunks {
			chunk := chunk
			p.Go(func() *BatchProcessResult {
				return processHistoryChunk(chunk, options)
			})
		}

		results := p.Wait()

		if err := updateDatabaseInBulk(db, results, &mu, result, options); err != nil {
			log.Error().Err(err).Msg("Failed to update database")
			return result, fmt.Errorf("failed to update database: %w", err)
		}

		// Update the trimmed counter for limit checking
		mu.Lock()
		recordsTrimmeddSoFar = result.TrimmedCount
		mu.Unlock()

		log.Info().
			Int64("total_processed", result.ProcessedCount).
			Int64("total_trimmed", result.TrimmedCount).
			Int64("bytes_saved", result.BytesSaved).
			Msg("Batch processing completed")

		// Check if we've reached the limit after updating
		if options.MaxRecords > 0 && recordsTrimmeddSoFar >= options.MaxRecords {
			log.Info().
				Int64("records_trimmed", recordsTrimmeddSoFar).
				Int64("max_records", options.MaxRecords).
				Msg("Reached maximum records limit, stopping")
			break
		}
	}

	log.Info().
		Int64("processed", result.ProcessedCount).
		Int64("trimmed", result.TrimmedCount).
		Int64("total", totalCount).
		Int64("bytes_saved", result.BytesSaved).
		Msg("History cleanup completed")

	return result, nil
}

type BatchProcessResult struct {
	Updates        []BatchUpdateItem
	ProcessedCount int64
	TrimmedCount   int64
	BytesSaved     int64
}

func chunkHistories(histories []History, chunkSize int) [][]History {
	var chunks [][]History
	for i := 0; i < len(histories); i += chunkSize {
		end := i + chunkSize
		if end > len(histories) {
			end = len(histories)
		}
		chunks = append(chunks, histories[i:end])
	}
	return chunks
}

func processHistoryChunk(histories []History, options HistoryCleanupOptions) *BatchProcessResult {
	result := &BatchProcessResult{
		ProcessedCount: int64(len(histories)),
	}

	for _, history := range histories {
		originalRequestSize := len(history.RawRequest)
		originalResponseSize := len(history.RawResponse)

		trimmed := false
		var savedBytes int64
		newRequest := history.RawRequest
		newResponse := history.RawResponse

		if originalRequestSize > options.MaxBodySize {
			if trimmedReq, bytes, err := trimHTTPMessage(history.RawRequest, options.MaxBodySize); err == nil && bytes > 0 {
				newRequest = trimmedReq
				savedBytes += bytes
				trimmed = true
			}
		}

		if originalResponseSize > options.MaxBodySize {
			if trimmedResp, bytes, err := trimHTTPMessage(history.RawResponse, options.MaxBodySize); err == nil && bytes > 0 {
				newResponse = trimmedResp
				savedBytes += bytes
				trimmed = true
			}
		}

		if trimmed {
			result.Updates = append(result.Updates, BatchUpdateItem{
				ID:          history.ID,
				RawRequest:  newRequest,
				RawResponse: newResponse,
				BytesSaved:  savedBytes,
			})
			result.TrimmedCount++
			result.BytesSaved += savedBytes
		}
	}

	return result
}

func trimHTTPMessage(message []byte, maxBodySize int) ([]byte, int64, error) {
	headers, body, err := SplitHTTPMessage(message)
	if err != nil {
		return nil, 0, err
	}

	if len(body) <= maxBodySize {
		return nil, 0, nil
	}

	truncationMsg := []byte("\n\n[... TRUNCATED ...]")
	newSize := len(headers) + maxBodySize + len(truncationMsg)

	trimmed := make([]byte, 0, newSize)
	trimmed = append(trimmed, headers...)
	trimmed = append(trimmed, body[:maxBodySize]...)
	trimmed = append(trimmed, truncationMsg...)

	bytesSaved := int64(len(message) - len(trimmed))
	return trimmed, bytesSaved, nil
}

func updateDatabaseInBulk(db *gorm.DB, results []*BatchProcessResult, mu *sync.Mutex, finalResult *HistoryCleanupResult, options HistoryCleanupOptions) error {
	var allUpdates []BatchUpdateItem
	var totalProcessed, totalTrimmed, totalBytesSaved int64

	for _, result := range results {
		allUpdates = append(allUpdates, result.Updates...)
		totalProcessed += result.ProcessedCount
		totalTrimmed += result.TrimmedCount
		totalBytesSaved += result.BytesSaved
	}

	mu.Lock()
	currentTrimmedCount := finalResult.TrimmedCount

	// Apply max records limit to the updates
	if options.MaxRecords > 0 {
		remainingSlots := options.MaxRecords - currentTrimmedCount
		if remainingSlots <= 0 {
			// Already reached the limit, don't process any more
			mu.Unlock()
			return nil
		}

		if int64(len(allUpdates)) > remainingSlots {
			// Trim the updates to fit within the limit
			allUpdates = allUpdates[:remainingSlots]
			totalTrimmed = remainingSlots

			// Recalculate bytes saved for the limited updates
			totalBytesSaved = 0
			for _, update := range allUpdates {
				totalBytesSaved += update.BytesSaved
			}

			log.Info().
				Int64("remaining_slots", remainingSlots).
				Int("limited_updates", len(allUpdates)).
				Msg("Limited updates to respect max-records")
		}
	}

	finalResult.ProcessedCount += totalProcessed
	finalResult.TrimmedCount += int64(len(allUpdates)) // Use actual updates applied
	finalResult.BytesSaved += totalBytesSaved
	mu.Unlock()

	if len(allUpdates) == 0 {
		return nil
	}

	return performOptimizedBulkUpdate(db, allUpdates)
}

func performOptimizedBulkUpdate(db *gorm.DB, updates []BatchUpdateItem) error {
	if len(updates) == 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		batchSize := 50
		for i := 0; i < len(updates); i += batchSize {
			end := i + batchSize
			if end > len(updates) {
				end = len(updates)
			}

			batch := updates[i:end]

			if err := executeBulkCaseUpdate(tx, batch); err != nil {
				return fmt.Errorf("failed to execute bulk update for batch starting at %d: %w", i, err)
			}
		}
		return nil
	})
}

// Uses individual updates in a transaction for PostgreSQL bytea compatibility
func executeBulkCaseUpdate(tx *gorm.DB, updates []BatchUpdateItem) error {
	if len(updates) == 0 {
		return nil
	}

	for _, update := range updates {
		result := tx.Model(&History{}).
			Where("id = ?", update.ID).
			Updates(map[string]interface{}{
				"raw_request":  update.RawRequest,
				"raw_response": update.RawResponse,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update record ID %d: %w", update.ID, result.Error)
		}
	}

	log.Debug().
		Int("batch_size", len(updates)).
		Msg("Executed bulk update")

	return nil
}

func performDryRunAnalysis(query *gorm.DB, totalCount int64, options HistoryCleanupOptions) (*HistoryCleanupResult, error) {
	result := &HistoryCleanupResult{
		DryRun:         true,
		ProcessedCount: totalCount,
	}

	sampleSize := int64(1000)
	if totalCount <= sampleSize {
		return analyzeAllRecords(query, totalCount, options)
	}

	log.Info().Int64("total_records", totalCount).Int64("sample_size", sampleSize).Msg("Using sampling for dry-run analysis")

	var histories []History
	if err := query.Limit(int(sampleSize)).Find(&histories).Error; err != nil {
		log.Error().Err(err).Msg("Failed to fetch sample for dry-run")
		return nil, fmt.Errorf("failed to fetch sample: %w", err)
	}

	sampleResults := iter.Map(histories, func(h *History) *SampleResult {
		return analyzeSingleRecord(*h, options)
	})

	var sampleTrimmed int64
	var sampleBytesSaved int64
	for _, sr := range sampleResults {
		if sr.BytesSaved > 0 {
			sampleTrimmed++
			sampleBytesSaved += sr.BytesSaved
		}
	}

	if len(histories) > 0 {
		trimRate := float64(sampleTrimmed) / float64(len(histories))
		avgBytesSaved := float64(sampleBytesSaved) / float64(len(histories))

		result.TrimmedCount = int64(float64(totalCount) * trimRate)
		result.BytesSaved = int64(float64(totalCount) * avgBytesSaved)
	}

	log.Info().
		Int64("sample_trimmed", sampleTrimmed).
		Int64("sample_bytes_saved", sampleBytesSaved).
		Int("sample_size", len(histories)).
		Float64("estimated_trim_rate", float64(result.TrimmedCount)/float64(totalCount)*100).
		Msg("Dry-run sampling analysis completed")

	return result, nil
}

type SampleResult struct {
	BytesSaved int64
}

func analyzeSingleRecord(history History, options HistoryCleanupOptions) *SampleResult {
	result := &SampleResult{}

	originalRequestSize := len(history.RawRequest)
	originalResponseSize := len(history.RawResponse)

	if originalRequestSize > options.MaxBodySize {
		if requestHeaders, requestBody, err := SplitHTTPMessage(history.RawRequest); err == nil && len(requestBody) > options.MaxBodySize {
			truncationMsg := []byte("\n\n[... TRUNCATED ...]")
			newSize := len(requestHeaders) + options.MaxBodySize + len(truncationMsg)
			result.BytesSaved += int64(originalRequestSize - newSize)
		}
	}

	if originalResponseSize > options.MaxBodySize {
		if responseHeaders, responseBody, err := SplitHTTPMessage(history.RawResponse); err == nil && len(responseBody) > options.MaxBodySize {
			truncationMsg := []byte("\n\n[... TRUNCATED ...]")
			newSize := len(responseHeaders) + options.MaxBodySize + len(truncationMsg)
			result.BytesSaved += int64(originalResponseSize - newSize)
		}
	}

	return result
}

func analyzeAllRecords(query *gorm.DB, totalCount int64, options HistoryCleanupOptions) (*HistoryCleanupResult, error) {
	result := &HistoryCleanupResult{
		DryRun:         true,
		ProcessedCount: totalCount,
	}

	var histories []History
	if err := query.Find(&histories).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch records for analysis: %w", err)
	}

	analysisResults := iter.Map(histories, func(h *History) *SampleResult {
		return analyzeSingleRecord(*h, options)
	})

	for _, ar := range analysisResults {
		if ar.BytesSaved > 0 {
			result.TrimmedCount++
			result.BytesSaved += ar.BytesSaved
		}
	}

	return result, nil
}
