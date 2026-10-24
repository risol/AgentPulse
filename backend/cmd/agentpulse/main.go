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
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
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
	launchDesktopByDefault := strings.EqualFold(
		strings.TrimSuffix(filepath.Base(os.Args[0]), filepath.Ext(os.Args[0])),
		"agentpulse-launcher",
	)
	launchDesktop := flag.Bool("launch-desktop", launchDesktopByDefault, "start the Flutter desktop UI after the Go services are ready")
	desktopExecutable := flag.String("desktop-executable", "", "optional path to the Flutter desktop executable")
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
	receiver, err := startReceiverRuntime(address, otlp.NewHandlerWithWriter(store, writeQueue, slog.Default(), changes))
	if err != nil {
		slog.Error("start OTLP receiver", "address", address, "error", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := receiver.Shutdown(shutdownCtx); err != nil {
			slog.Warn("shut down receiver", "error", err)
		}
	}()
	slog.Info("OTLP receiver ready", "address", receiver.Address(), "database", *databasePath)
	otlpEndpoint, err := endpointForAddress(receiver.Address())
	if err != nil {
		slog.Error("resolve OTLP endpoint", "error", err)
		os.Exit(1)
	}
	endpoint := &mutableEndpoint{value: otlpEndpoint}
	api, err := appapi.Start(appapi.Dependencies{
		Process:            application.NewProcessService(store, processMonitor),
		SessionQueries:     application.NewSessionQueryService(store),
		SessionAnalytics:   application.NewSessionAnalyticsService(store),
		Trends:             application.NewAnalyticsTrendService(store),
		Retention:          application.NewTelemetryRetentionService(store),
		IngestDiagnostics:  application.NewIngestDiagnosticService(store),
		TelemetryOverview:  application.NewTelemetryOverviewService(store),
		TelemetryData:      application.NewTelemetryDataService(store),
		CodexConfiguration: codexconfig.Commands{EndpointProvider: endpoint.Load},
		IngestQueue:        writeQueue,
		BackendVersion:     version,
		ReceiverAddress:    receiver.Address(),
		ReceiverSettings:   receiverSettings,
		CheckReceiverPort:  checkReceiverPortAvailability,
		ChangeReceiverPort: func(port int) (string, error) {
			address, err := receiver.ChangePort(port)
			if err != nil {
				return "", err
			}
			nextEndpoint, err := endpointForAddress(address)
			if err != nil {
				return "", err
			}
			endpoint.Store(nextEndpoint)
			slog.Info("OTLP receiver switched to new port", "address", address)
			return address, nil
		},
		LogCleaner: logManager,
		Changes:    changes,
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
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := receiver.Shutdown(shutdownCtx); err != nil {
			slog.Error("shut down receiver", "error", err)
		}
	}()
	if *handshake {
		if err := json.NewEncoder(os.Stdout).Encode(map[string]any{"api_version": 1, "backend_version": version, "api_address": api.Address(), "api_token": api.Token(), "otlp_address": receiver.Address()}); err != nil {
			slog.Error("write startup handshake", "error", err)
			os.Exit(1)
		}
		go func() {
			_, _ = io.Copy(io.Discard, os.Stdin)
			stop()
		}()
	}

	var desktopProcess *os.Process
	var desktopDone <-chan struct{}
	if *launchDesktop {
		executable, err := resolveDesktopExecutable(*desktopExecutable)
		if err != nil {
			slog.Error("locate desktop UI", "error", err)
			return
		}
		command, err := startDesktopProcess(executable, api.Address(), api.Token(), version)
		if err != nil {
			slog.Error("start desktop UI", "error", err)
			return
		}
		desktopProcess = command.Process
		done := make(chan struct{})
		desktopDone = done
		go func() {
			if err := command.Wait(); err != nil && ctx.Err() == nil {
				slog.Warn("desktop UI exited with an error", "error", err)
			}
			close(done)
			stop()
		}()
	}
	if desktopProcess != nil {
		defer func() {
			select {
			case <-desktopDone:
			default:
				_ = desktopProcess.Kill()
				<-desktopDone
			}
		}()
	}

	select {
	case <-ctx.Done():
	case err := <-receiver.Errors():
		if err != nil {
			slog.Error("serve OTLP requests", "error", err)
			stop()
		}
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

func endpointForAddress(address string) (string, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return "", errors.New("OTLP listener has an invalid TCP address")
	}
	parsedPort, err := strconv.Atoi(port)
	if err != nil || parsedPort < 1 || parsedPort > 65535 {
		return "", errors.New("OTLP listener has an invalid TCP port")
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(parsedPort)), nil
}

func checkReceiverPortAvailability(port int) error {
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return listener.Close()
}

type mutableEndpoint struct {
	mu    sync.RWMutex
	value string
}

func (endpoint *mutableEndpoint) Load() string {
	endpoint.mu.RLock()
	defer endpoint.mu.RUnlock()
	return endpoint.value
}

func (endpoint *mutableEndpoint) Store(value string) {
	endpoint.mu.Lock()
	endpoint.value = value
	endpoint.mu.Unlock()
}
