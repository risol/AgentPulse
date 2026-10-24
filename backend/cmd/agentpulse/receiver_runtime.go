package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type receiverRuntime struct {
	mu       sync.RWMutex
	handler  http.Handler
	listener net.Listener
	server   *http.Server
	errors   chan error
	closed   bool
}

func startReceiverRuntime(address string, handler http.Handler) (*receiverRuntime, error) {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, err
	}
	runtime := &receiverRuntime{
		handler:  handler,
		listener: listener,
		server:   newReceiverHTTPServer(handler),
		errors:   make(chan error, 1),
	}
	runtime.serve(runtime.server, listener)
	return runtime, nil
}

func (runtime *receiverRuntime) Address() string {
	runtime.mu.RLock()
	defer runtime.mu.RUnlock()
	return runtime.listener.Addr().String()
}

func (runtime *receiverRuntime) ChangePort(port int) (string, error) {
	runtime.mu.Lock()
	if runtime.closed {
		runtime.mu.Unlock()
		return "", errors.New("OTLP receiver is shutting down")
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		runtime.mu.Unlock()
		return "", err
	}
	server := newReceiverHTTPServer(runtime.handler)
	oldServer := runtime.server
	runtime.listener = listener
	runtime.server = server
	runtime.mu.Unlock()

	runtime.serve(server, listener)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := oldServer.Shutdown(shutdownCtx); err != nil {
		_ = oldServer.Close()
	}
	return listener.Addr().String(), nil
}

func (runtime *receiverRuntime) Shutdown(ctx context.Context) error {
	runtime.mu.Lock()
	if runtime.closed {
		runtime.mu.Unlock()
		return nil
	}
	runtime.closed = true
	server := runtime.server
	runtime.mu.Unlock()
	return server.Shutdown(ctx)
}

func (runtime *receiverRuntime) Errors() <-chan error { return runtime.errors }

func (runtime *receiverRuntime) serve(server *http.Server, listener net.Listener) {
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case runtime.errors <- err:
			default:
			}
		}
	}()
}

func newReceiverHTTPServer(handler http.Handler) *http.Server {
	return &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}
