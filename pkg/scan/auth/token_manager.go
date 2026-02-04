package auth

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/ohler55/ojg/jp"
	"github.com/ohler55/ojg/oj"
	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

const maxRefreshResponseSize = 1 << 20 // 1 MB

type refreshBatch struct {
	done    chan struct{}
	outcome refreshOutcome
}

type tokenState struct {
	mu        sync.Mutex
	token     string
	fetchedAt time.Time
	stopCh    chan struct{}
	refCount  atomic.Int32
	running   atomic.Bool

	refreshMu    sync.Mutex
	currentBatch *refreshBatch
}

type refreshOutcome struct {
	token string
	err   error
}

type TokenManager struct {
	dbConn *db.DatabaseConnection
	mu     sync.RWMutex
	states map[uuid.UUID]*tokenState
}

func NewTokenManager(dbConn *db.DatabaseConnection) *TokenManager {
	return &TokenManager{
		dbConn: dbConn,
		states: make(map[uuid.UUID]*tokenState),
	}
}

func (tm *TokenManager) getOrCreateState(authConfigID uuid.UUID) *tokenState {
	tm.mu.RLock()
	state, exists := tm.states[authConfigID]
	tm.mu.RUnlock()

	if exists {
		return state
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if state, exists = tm.states[authConfigID]; exists {
		return state
	}

	state = &tokenState{
		stopCh: make(chan struct{}),
	}
	tm.states[authConfigID] = state
	return state
}

func (tm *TokenManager) GetToken(authConfigID uuid.UUID) (string, error) {
	state := tm.getOrCreateState(authConfigID)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.token != "" {
		return state.token, nil
	}

	config, err := tm.dbConn.GetTokenRefreshConfigByAuthConfigID(authConfigID)
	if err != nil {
		return "", fmt.Errorf("fetching token refresh config for auth config %s: %w", authConfigID, err)
	}

	if config.CurrentToken != "" {
		state.token = config.CurrentToken
		if config.TokenFetchedAt != nil {
			state.fetchedAt = *config.TokenFetchedAt
		}
		return state.token, nil
	}

	token, err := tm.ExecuteRefresh(config)
	if err != nil {
		return "", fmt.Errorf("executing initial token refresh for auth config %s: %w", authConfigID, err)
	}

	now := time.Now()
	if dbErr := tm.dbConn.UpdateTokenRefreshState(config.ID, token, now, ""); dbErr != nil {
		log.Error().Err(dbErr).Str("auth_config_id", authConfigID.String()).Msg("Failed to persist initial token to database")
	}

	state.token = token
	state.fetchedAt = now
	return token, nil
}

func (tm *TokenManager) ForceRefresh(authConfigID uuid.UUID) (string, error) {
	state := tm.getOrCreateState(authConfigID)

	state.refreshMu.Lock()
	if state.currentBatch != nil {
		batch := state.currentBatch
		state.refreshMu.Unlock()
		<-batch.done
		return batch.outcome.token, batch.outcome.err
	}

	batch := &refreshBatch{done: make(chan struct{})}
	state.currentBatch = batch
	state.refreshMu.Unlock()

	token, err := tm.doRefresh(authConfigID)

	batch.outcome = refreshOutcome{token: token, err: err}
	close(batch.done)

	state.refreshMu.Lock()
	state.currentBatch = nil
	state.refreshMu.Unlock()

	return token, err
}

func (tm *TokenManager) doRefresh(authConfigID uuid.UUID) (string, error) {
	config, err := tm.dbConn.GetTokenRefreshConfigByAuthConfigID(authConfigID)
	if err != nil {
		return "", fmt.Errorf("fetching token refresh config for auth config %s: %w", authConfigID, err)
	}

	token, err := tm.ExecuteRefresh(config)
	if err != nil {
		if dbErr := tm.dbConn.UpdateTokenRefreshState(config.ID, "", time.Time{}, err.Error()); dbErr != nil {
			log.Error().Err(dbErr).Str("auth_config_id", authConfigID.String()).Msg("Failed to persist refresh error to database")
		}
		return "", fmt.Errorf("token refresh failed for auth config %s: %w", authConfigID, err)
	}

	now := time.Now()
	if dbErr := tm.dbConn.UpdateTokenRefreshState(config.ID, token, now, ""); dbErr != nil {
		log.Error().Err(dbErr).Str("auth_config_id", authConfigID.String()).Msg("Failed to persist refreshed token to database")
	}

	state := tm.getOrCreateState(authConfigID)
	state.mu.Lock()
	state.token = token
	state.fetchedAt = now
	state.mu.Unlock()

	return token, nil
}

func (tm *TokenManager) RegisterScan(authConfigID uuid.UUID) {
	state := tm.getOrCreateState(authConfigID)
	prev := state.refCount.Add(1) - 1

	if prev == 0 {
		go tm.startRefreshLoop(authConfigID, state)
	}

	log.Info().Str("auth_config_id", authConfigID.String()).Int32("ref_count", state.refCount.Load()).Msg("Scan registered for token refresh")
}

func (tm *TokenManager) UnregisterScan(authConfigID uuid.UUID) {
	tm.mu.RLock()
	state, exists := tm.states[authConfigID]
	tm.mu.RUnlock()

	if !exists {
		log.Warn().Str("auth_config_id", authConfigID.String()).Msg("Attempted to unregister scan for unknown auth config")
		return
	}

	current := state.refCount.Add(-1)
	log.Info().Str("auth_config_id", authConfigID.String()).Int32("ref_count", current).Msg("Scan unregistered from token refresh")

	if current <= 0 {
		if state.running.CompareAndSwap(true, false) {
			close(state.stopCh)
		}
		tm.mu.Lock()
		delete(tm.states, authConfigID)
		tm.mu.Unlock()
	}
}

func (tm *TokenManager) ExecuteRefresh(config *db.TokenRefreshConfig) (string, error) {
	var bodyReader io.Reader
	if config.RequestBody != "" {
		bodyReader = strings.NewReader(config.RequestBody)
	}

	req, err := http.NewRequest(config.RequestMethod, config.RequestURL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("building refresh request: %w", err)
	}

	if config.RequestContentType != "" {
		req.Header.Set("Content-Type", config.RequestContentType)
	}

	for key, value := range config.RequestHeaders {
		req.Header.Set(key, value)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("executing refresh request to %s: %w", config.RequestURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("token refresh request returned status %d", resp.StatusCode)
	}

	switch config.ExtractionSource {
	case db.TokenExtractionSourceBodyJSONPath:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxRefreshResponseSize))
		if err != nil {
			return "", fmt.Errorf("reading refresh response body: %w", err)
		}
		return extractTokenFromJSONPath(body, config.ExtractionValue)

	case db.TokenExtractionSourceResponseHeader:
		headerValue := resp.Header.Get(config.ExtractionValue)
		if headerValue == "" {
			return "", fmt.Errorf("response header %q is empty or not present", config.ExtractionValue)
		}
		return headerValue, nil

	default:
		return "", fmt.Errorf("unsupported extraction source: %s", config.ExtractionSource)
	}
}

func (tm *TokenManager) Stop() {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	for authConfigID, state := range tm.states {
		if state.running.CompareAndSwap(true, false) {
			close(state.stopCh)
			log.Info().Str("auth_config_id", authConfigID.String()).Msg("Stopped token refresh loop")
		}
	}
}

func (tm *TokenManager) startRefreshLoop(authConfigID uuid.UUID, state *tokenState) {
	if !state.running.CompareAndSwap(false, true) {
		return
	}

	log.Info().Str("auth_config_id", authConfigID.String()).Msg("Starting proactive token refresh loop")

	config, err := tm.dbConn.GetTokenRefreshConfigByAuthConfigID(authConfigID)
	if err != nil {
		log.Error().Err(err).Str("auth_config_id", authConfigID.String()).Msg("Failed to fetch config for refresh loop")
		state.running.Store(false)
		return
	}

	interval := time.Duration(config.IntervalSeconds) * time.Second
	if interval <= 0 {
		log.Warn().Str("auth_config_id", authConfigID.String()).Int("interval_seconds", config.IntervalSeconds).Msg("Invalid refresh interval, stopping loop")
		state.running.Store(false)
		return
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-state.stopCh:
			log.Info().Str("auth_config_id", authConfigID.String()).Msg("Token refresh loop stopped")
			return
		case <-timer.C:
			token, err := tm.ForceRefresh(authConfigID)
			if err != nil {
				log.Error().Err(err).Str("auth_config_id", authConfigID.String()).Msg("Proactive token refresh failed")
			} else {
				log.Info().Str("auth_config_id", authConfigID.String()).Int("token_length", len(token)).Msg("Proactive token refresh succeeded")
			}
			timer.Reset(interval)
		}
	}
}

func extractTokenFromJSONPath(body []byte, jsonPath string) (string, error) {
	obj, err := oj.Parse(body)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSON response: %w", err)
	}

	expr, err := jp.ParseString(jsonPath)
	if err != nil {
		return "", fmt.Errorf("failed to parse JSONPath expression %q: %w", jsonPath, err)
	}

	results := expr.Get(obj)
	if len(results) == 0 {
		return "", fmt.Errorf("JSONPath %q matched no values in response", jsonPath)
	}

	token, ok := results[0].(string)
	if !ok {
		return "", fmt.Errorf("JSONPath %q matched a non-string value: %v", jsonPath, results[0])
	}

	return token, nil
}
