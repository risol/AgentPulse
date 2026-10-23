package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/agentpulse/agentpulse/backend/internal/appapi"
	"github.com/agentpulse/agentpulse/backend/internal/application"
	"github.com/agentpulse/agentpulse/backend/internal/applog"
	"github.com/agentpulse/agentpulse/backend/internal/codexadapter"
	"github.com/agentpulse/agentpulse/backend/internal/codexconfig"
	"github.com/agentpulse/agentpulse/backend/internal/ingest"
	"github.com/agentpulse/agentpulse/backend/internal/otlp"
	"github.com/agentpulse/agentpulse/backend/internal/process"
	"github.com/agentpulse/agentpulse/backend/internal/storage"
	"github.com/agentpulse/agentpulse/backend/internal/telemetry"
	_ "modernc.org/sqlite"
)

var version = "development"

func main() {
	addressOverride := flag.String("otlp-address", "", "optional loopback OTLP/HTTP listen address override")
	databasePath := flag.String("db", defaultDatabasePath(), "SQLite database path")
	handshake := flag.Bool("handshake-stdout", false, "write the private local API startup handshake to stdout")
	flag.Parse()
	logManager, logErr := applog.OpenDefault()
	if logErr == nil {
		slog.SetDefault(logManager.Slog())
		defer logManager.Close()
	} else {
		slog.Warn("file logging unavailable; using standard error output")
	}

	if err := os.MkdirAll(filepath.Dir(*databasePath), 0o700); err != nil {
		slog.Error("create data directory", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(*databasePath)+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)")
	if err != nil {
		slog.Error("open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()
	storage.ConfigureConnectionPool(db)
	if err := storage.Initialize(context.Background(), db); err != nil {
		slog.Error("initialize database", "error", err)
		os.Exit(1)
	}
	store := storage.NewSQLite(db)
	receiverSettings := application.NewReceiverSettingsService(store)
	configuredReceiver, err := receiverSettings.Load(context.Background())
	if err != nil {
		slog.Error("load receiver settings", "error", err)
		os.Exit(1)
	}
	address := *addressOverride
	if address == "" {
		address = net.JoinHostPort("127.0.0.1", strconv.Itoa(configuredReceiver.Port))
	}
	if !isLoopbackAddress(address) {
		slog.Error("OTLP receiver must bind to a loopback address", "address", address)
		os.Exit(2)
	}
	processConfig, err := store.LoadProcessMonitorConfig(context.Background())
	if err != nil {
		slog.Error("load process monitor settings", "error", err)
		os.Exit(1)
	}
	processMonitor, err := process.NewMonitor(store, process.NewCollector(), processConfig)
	if err != nil {
		slog.Error("initialize process monitor", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	writeQueue := ingest.NewQueue(codexadapter.NewWriter(store), 4)
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := writeQueue.Close(closeCtx); err != nil {
			slog.Warn("telemetry write queue did not drain before shutdown", "error", err)
		}
	}()
	changes := telemetry.NewChangeBus()
	server := &http.Server{
		Addr:              address,
		Handler:           otlp.NewHandlerWithWriter(store, writeQueue, slog.Default(), changes),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		slog.Error("start OTLP receiver", "address", address, "error", err)
		os.Exit(1)
	}
	defer listener.Close()
	slog.Info("OTLP receiver ready", "address", listener.Addr().String(), "database", *databasePath)
	otlpEndpoint, err := endpointForListener(listener)
	if err != nil {
		slog.Error("resolve OTLP endpoint", "error", err)
		os.Exit(1)
	}
	api, err := appapi.Start(appapi.Dependencies{
		Process:            application.NewProcessService(store, processMonitor),
		SessionQueries:     application.NewSessionQueryService(store),
		SessionAnalytics:   application.NewSessionAnalyticsService(store),
		Trends:             application.NewAnalyticsTrendService(store),
		Retention:          application.NewTelemetryRetentionService(store),
		IngestDiagnostics:  application.NewIngestDiagnosticService(store),
		TelemetryOverview:  application.NewTelemetryOverviewService(store),
		TelemetryData:      application.NewTelemetryDataService(store),
		CodexConfiguration: codexconfig.Commands{Endpoint: otlpEndpoint},
		IngestQueue:        writeQueue,
		BackendVersion:     version,
		ReceiverAddress:    listener.Addr().String(),
		ReceiverSettings:   receiverSettings,
		CheckReceiverPort:  checkReceiverPortAvailability,
		LogCleaner:         logManager,
		Changes:            changes,
	})
	if err != nil {
		slog.Error("start application API", "error", err)
		os.Exit(1)
	}
	defer api.Close()
	processMonitor.Start(ctx)
	api.StartRetention(ctx)
	defer func() {
		stop()
		processMonitor.Wait()
		api.WaitRetention()
	}()
	if *handshake {
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"api_version": 1, "backend_version": version, "api_address": api.Address(), "api_token": api.Token(), "otlp_address": listener.Addr().String()}); err != nil {
			slog.Error("write startup handshake", "error", err)
			os.Exit(1)
		}
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shut down receiver", "error", err)
		}
	}()
	if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("serve OTLP requests", "error", err)
		os.Exit(1)
	}
}

func defaultDatabasePath() string {
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "AgentPulse", "agentpulse.db")
	}
	return "agentpulse.db"
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func endpointForListener(listener net.Listener) (string, error) {
	address, okay := listener.Addr().(*net.TCPAddr)
	if !okay || address.Port < 1 || address.Port > 65535 {
		return "", errors.New("OTLP listener has an invalid TCP address")
	}
	return "http://" + net.JoinHostPort(address.IP.String(), strconv.Itoa(address.Port)), nil
}

func checkReceiverPortAvailability(port int) error {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return listener.Close()
}
