// Command perfcheck measures OTLP ingest, dashboard refresh, history paging,
// and concurrent query/write behavior against an isolated synthetic database.
// It never opens the user's AgentPulse database.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/appapi"
	"github.com/agentpulse/agentpulse/backend/internal/application"
	"github.com/agentpulse/agentpulse/backend/internal/codexadapter"
	"github.com/agentpulse/agentpulse/backend/internal/codexconfig"
	"github.com/agentpulse/agentpulse/backend/internal/ingest"
	"github.com/agentpulse/agentpulse/backend/internal/otlp"
	"github.com/agentpulse/agentpulse/backend/internal/storage"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	collectorlogs "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	common "go.opentelemetry.io/proto/otlp/common/v1"
	logs "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

func main() {
	eventCount := flag.Int("events", 100000, "number of synthetic history events to seed")
	samples := flag.Int("samples", 40, "number of concurrent dashboard refresh samples")
	rateDuration := flag.Duration("rate-duration", 5*time.Second, "duration of each OTLP rate sample")
	flag.Parse()
	if *eventCount < 1000 || *samples < 10 || *rateDuration < time.Second {
		fmt.Fprintln(os.Stderr, "events must be at least 1000, samples at least 10, and rate-duration at least 1s")
		os.Exit(2)
	}
	if err := run(*eventCount, *samples, *rateDuration); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(eventCount, samples int, rateDuration time.Duration) error {
	ctx := context.Background()
	tempDir, err := os.MkdirTemp("", "agentpulse-perfcheck-")
	if err != nil {
		return fmt.Errorf("create isolated performance directory: %w", err)
	}
	defer os.RemoveAll(tempDir)
	databasePath := tempDir + string(os.PathSeparator) + "synthetic.db"
	db, err := sql.Open("sqlite", "file:"+strings.ReplaceAll(databasePath, `\`, "/")+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		return fmt.Errorf("open synthetic database: %w", err)
	}
	defer db.Close()
	storage.ConfigureConnectionPool(db)
	if err := storage.Initialize(ctx, db); err != nil {
		return fmt.Errorf("initialize synthetic database: %w", err)
	}
	store := storage.NewSQLite(db)
	seedStarted := time.Now()
	if err := seedHistory(ctx, store, eventCount); err != nil {
		return fmt.Errorf("seed synthetic history: %w", err)
	}
	seedDuration := time.Since(seedStarted)

	queue := ingest.NewQueue(codexadapter.NewWriter(store), 4)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = queue.Close(closeCtx)
	}()
	changes := telemetry.NewChangeBus()
	receiverListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen for synthetic OTLP load: %w", err)
	}
	receiver := &http.Server{
		Handler: otlp.NewHandlerWithWriter(store, queue, slog.New(slog.NewTextHandler(io.Discard, nil)), changes),
	}
	go func() { _ = receiver.Serve(receiverListener) }()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = receiver.Shutdown(shutdownCtx)
		_ = receiverListener.Close()
	}()

	api, err := appapi.Start(appapi.Dependencies{
		Process:            application.NewProcessService(store, nil),
		SessionQueries:     application.NewSessionQueryService(store),
		SessionAnalytics:   application.NewSessionAnalyticsService(store),
		Trends:             application.NewAnalyticsTrendService(store),
		Retention:          application.NewTelemetryRetentionService(store),
		IngestDiagnostics:  application.NewIngestDiagnosticService(store),
		TelemetryOverview:  application.NewTelemetryOverviewService(store),
		TelemetryData:      application.NewTelemetryDataService(store),
		ReceiverSettings:   application.NewReceiverSettingsService(store),
		CodexConfiguration: codexconfig.Commands{Endpoint: "http://127.0.0.1:4318"},
		IngestQueue:        queue,
		BackendVersion:     "perfcheck",
		ReceiverAddress:    receiverListener.Addr().String(),
		CheckReceiverPort:  func(int) error { return nil },
		Changes:            changes,
	})
	if err != nil {
		return fmt.Errorf("start isolated query API: %w", err)
	}
	defer api.Close()

	ingestResults, err := measureIngestRates(receiverListener.Addr().String(), rateDuration)
	if err != nil {
		return err
	}
	refreshResult, err := measureRefresh(api.Address(), api.Token(), samples)
	if err != nil {
		return err
	}
	paginationResult, err := measureUnassignedPagination(api.Address(), api.Token(), samples)
	if err != nil {
		return err
	}
	concurrentResult, err := measureConcurrentQueriesAndIngest(
		receiverListener.Addr().String(), api.Address(), api.Token(), rateDuration, samples,
	)
	if err != nil {
		return err
	}
	output := struct {
		Database                      string             `json:"database"`
		SeededEvents                  int                `json:"seeded_events"`
		SeedDurationMS                int64              `json:"seed_duration_ms"`
		IngestRates                   []ingestRateResult `json:"ingest_rates"`
		DashboardRefresh              latencyResult      `json:"dashboard_refresh"`
		EventPagination               latencyResult      `json:"event_pagination"`
		ConcurrentDashboardRefresh    latencyResult      `json:"concurrent_dashboard_refresh"`
		ConcurrentIngestRates         []ingestRateResult `json:"concurrent_ingest_rates"`
		RefreshP95TargetMS            int64              `json:"refresh_p95_target_ms"`
		RefreshP95TargetMet           bool               `json:"refresh_p95_target_met"`
		ConcurrentRefreshP95TargetMet bool               `json:"concurrent_refresh_p95_target_met"`
	}{
		Database: "temporary synthetic SQLite (removed at exit)", SeededEvents: eventCount,
		SeedDurationMS: seedDuration.Milliseconds(), IngestRates: ingestResults,
		DashboardRefresh: refreshResult, EventPagination: paginationResult,
		ConcurrentDashboardRefresh: concurrentResult.DashboardRefresh,
		ConcurrentIngestRates:      concurrentResult.IngestRates,
		RefreshP95TargetMS:         1000, RefreshP95TargetMet: refreshResult.P95MS < 1000,
		ConcurrentRefreshP95TargetMet: concurrentResult.DashboardRefresh.P95MS < 1000,
	}
	encoded, _ := json.MarshalIndent(output, "", "  ")
	fmt.Println(string(encoded))
	return nil
}

func seedHistory(ctx context.Context, store *storage.SQLite, count int) error {
	writer := codexadapter.NewWriter(store)
	const batchSize = 500
	now := time.Now().UnixNano()
	for offset := 0; offset < count; offset += batchSize {
		end := min(offset+batchSize, count)
		events := make([]telemetry.Event, 0, end-offset)
		for index := offset; index < end; index++ {
			timestamp := now - int64(index%86400)*int64(time.Second)
			attributes := map[string]any{
				"model":                 "synthetic-model",
				"input_token_count":     int64(100 + index%20),
				"output_token_count":    int64(40 + index%15),
				"reasoning_token_count": int64(index % 5),
			}
			if index%10 != 1 {
				attributes["conversation.id"] = fmt.Sprintf("session-%d", index/10)
				attributes["turn.id"] = fmt.Sprintf("turn-%d", index/100)
			}
			if index%20 == 0 {
				attributes["tool_name"] = []string{"shell", "test", "build"}[index/20%3]
				attributes["tool.call_id"] = fmt.Sprintf("tool-%d", index)
				attributes["success"] = index%7 != 0
				attributes["duration_ms"] = float64(25 + index%500)
			}
			events = append(events, telemetry.Event{EventTimeNS: &timestamp, Name: "synthetic_event", Attributes: attributes})
		}
		if err := writer.SaveTelemetry(ctx, telemetry.Batch{
			SourceID: telemetry.SourceCodex, Signal: telemetry.SignalLogs,
			ReceivedAtNS: now, Events: events,
		}); err != nil {
			return fmt.Errorf("batch ending at event %d: %w", end, err)
		}
	}
	return nil
}

type ingestRateResult struct {
	TargetEventsPerSecond int     `json:"target_events_per_second"`
	AcceptedEvents        int     `json:"accepted_events"`
	DurationMS            int64   `json:"duration_ms"`
	ActualEventsPerSecond float64 `json:"actual_events_per_second"`
}

func measureIngestRates(receiverAddress string, duration time.Duration) ([]ingestRateResult, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	endpoint := "http://" + receiverAddress + "/v1/logs"
	var results []ingestRateResult
	sequence := 0
	for _, targetRate := range []int{10, 100, 1000} {
		eventsPerRequest := targetRate / 10
		interval := 100 * time.Millisecond
		started := time.Now()
		deadline := started.Add(duration)
		nextRequest := started
		accepted := 0
		for time.Now().Before(deadline) {
			records := make([]*logs.LogRecord, 0, eventsPerRequest)
			for range eventsPerRequest {
				sequence++
				records = append(records, &logs.LogRecord{
					TimeUnixNano: uint64(time.Now().UnixNano()),
					Attributes: []*common.KeyValue{
						stringAttribute("conversation.id", fmt.Sprintf("load-session-%d", sequence/10)),
						stringAttribute("model", "load-model"),
						intAttribute("output_token_count", int64(sequence%60)),
					},
				})
			}
			encoded, err := proto.Marshal(&collectorlogs.ExportLogsServiceRequest{
				ResourceLogs: []*logs.ResourceLogs{{ScopeLogs: []*logs.ScopeLogs{{LogRecords: records}}}},
			})
			if err != nil {
				return nil, fmt.Errorf("encode synthetic OTLP batch: %w", err)
			}
			request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(encoded))
			if err != nil {
				return nil, err
			}
			request.Header.Set("Content-Type", "application/x-protobuf")
			response, err := client.Do(request)
			if err != nil {
				return nil, fmt.Errorf("send %d events/s sample: %w", targetRate, err)
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			if response.StatusCode != http.StatusOK {
				return nil, fmt.Errorf("OTLP receiver rejected %d events/s sample with HTTP %d", targetRate, response.StatusCode)
			}
			accepted += len(records)
			nextRequest = nextRequest.Add(interval)
			if wait := time.Until(nextRequest); wait > 0 {
				time.Sleep(wait)
			}
		}
		elapsed := time.Since(started)
		results = append(results, ingestRateResult{
			TargetEventsPerSecond: targetRate, AcceptedEvents: accepted,
			DurationMS: elapsed.Milliseconds(), ActualEventsPerSecond: float64(accepted) / elapsed.Seconds(),
		})
	}
	return results, nil
}

type latencyResult struct {
	Samples int   `json:"samples"`
	P50MS   int64 `json:"p50_ms"`
	P95MS   int64 `json:"p95_ms"`
	MaxMS   int64 `json:"max_ms"`
}

type concurrentLoadResult struct {
	IngestRates      []ingestRateResult
	DashboardRefresh latencyResult
}

func measureConcurrentQueriesAndIngest(
	receiverAddress, apiAddress, token string, rateDuration time.Duration, sampleCount int,
) (concurrentLoadResult, error) {
	start := make(chan struct{})
	var wait sync.WaitGroup
	var result concurrentLoadResult
	var ingestErr, refreshErr error
	wait.Add(2)
	go func() {
		defer wait.Done()
		<-start
		result.IngestRates, ingestErr = measureIngestRates(receiverAddress, rateDuration)
	}()
	go func() {
		defer wait.Done()
		<-start
		result.DashboardRefresh, refreshErr = measureRefresh(apiAddress, token, sampleCount)
	}()
	close(start)
	wait.Wait()
	if ingestErr != nil {
		return concurrentLoadResult{}, fmt.Errorf("ingest during historical queries: %w", ingestErr)
	}
	if refreshErr != nil {
		return concurrentLoadResult{}, fmt.Errorf("historical queries during ingest: %w", refreshErr)
	}
	return result, nil
}

func measureRefresh(address, token string, sampleCount int) (latencyResult, error) {
	paths := []string{
		"/api/v1/dashboard/summary?range=24h",
		"/api/v1/analytics/trends?range=24h",
		"/api/v1/sessions?limit=25",
		"/api/v1/events/unassigned?limit=25",
		"/api/v1/tools/summary?range=24h",
		"/api/v1/tools/calls?range=24h&limit=25",
	}
	baseURL := "http://" + address
	client := &http.Client{Timeout: 30 * time.Second}
	latencies := make([]time.Duration, 0, sampleCount)
	for sample := 0; sample < sampleCount+5; sample++ {
		started := time.Now()
		results := make(chan error, len(paths))
		for _, path := range paths {
			go func(path string) {
				request, err := http.NewRequest(http.MethodGet, baseURL+path, nil)
				if err != nil {
					results <- err
					return
				}
				request.Header.Set("Authorization", "Bearer "+token)
				response, err := client.Do(request)
				if err != nil {
					results <- err
					return
				}
				_, _ = io.Copy(io.Discard, response.Body)
				response.Body.Close()
				if response.StatusCode != http.StatusOK {
					results <- fmt.Errorf("%s returned HTTP %d", path, response.StatusCode)
					return
				}
				results <- nil
			}(path)
		}
		for range paths {
			if err := <-results; err != nil {
				return latencyResult{}, err
			}
		}
		elapsed := time.Since(started)
		if sample >= 5 {
			latencies = append(latencies, elapsed)
		}
	}
	sort.Slice(latencies, func(left, right int) bool { return latencies[left] < latencies[right] })
	return latencyResult{
		Samples: len(latencies), P50MS: latencies[(len(latencies)-1)/2].Milliseconds(),
		P95MS: latencies[int(math.Ceil(float64(len(latencies))*0.95))-1].Milliseconds(),
		MaxMS: latencies[len(latencies)-1].Milliseconds(),
	}, nil
}

func measureUnassignedPagination(address, token string, sampleCount int) (latencyResult, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	baseURL := "http://" + address
	request, err := http.NewRequest(http.MethodGet, baseURL+"/api/v1/events/unassigned?limit=25", nil)
	if err != nil {
		return latencyResult{}, err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := client.Do(request)
	if err != nil {
		return latencyResult{}, fmt.Errorf("request first unassigned-event page: %w", err)
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return latencyResult{}, fmt.Errorf("first unassigned-event page returned HTTP %d", response.StatusCode)
	}
	var page struct {
		NextCursor *string `json:"next_cursor"`
	}
	err = json.NewDecoder(response.Body).Decode(&page)
	response.Body.Close()
	if err != nil {
		return latencyResult{}, fmt.Errorf("decode unassigned-event cursor: %w", err)
	}
	if page.NextCursor == nil {
		return latencyResult{}, fmt.Errorf("synthetic history did not produce a second unassigned-event page")
	}
	pageURL := baseURL + "/api/v1/events/unassigned?limit=25&cursor=" + url.QueryEscape(*page.NextCursor)
	latencies := make([]time.Duration, 0, sampleCount)
	for range sampleCount {
		started := time.Now()
		request, err := http.NewRequest(http.MethodGet, pageURL, nil)
		if err != nil {
			return latencyResult{}, err
		}
		request.Header.Set("Authorization", "Bearer "+token)
		response, err := client.Do(request)
		if err != nil {
			return latencyResult{}, fmt.Errorf("request paged unassigned events: %w", err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return latencyResult{}, fmt.Errorf("paged unassigned events returned HTTP %d", response.StatusCode)
		}
		latencies = append(latencies, time.Since(started))
	}
	sort.Slice(latencies, func(left, right int) bool { return latencies[left] < latencies[right] })
	return latencyResult{
		Samples: len(latencies), P50MS: latencies[(len(latencies)-1)/2].Milliseconds(),
		P95MS: latencies[int(math.Ceil(float64(len(latencies))*0.95))-1].Milliseconds(),
		MaxMS: latencies[len(latencies)-1].Milliseconds(),
	}, nil
}

func stringAttribute(key, value string) *common.KeyValue {
	return &common.KeyValue{Key: key, Value: &common.AnyValue{Value: &common.AnyValue_StringValue{StringValue: value}}}
}

func intAttribute(key string, value int64) *common.KeyValue {
	return &common.KeyValue{Key: key, Value: &common.AnyValue{Value: &common.AnyValue_IntValue{IntValue: value}}}
}
