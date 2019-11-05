// Package serviceapi provides a container compatible interface
package serviceapi

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/containers/libpod/libpod"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/coreos/go-systemd/activation"
)

// See https://docs.docker.com/engine/api/v1.40/
const (
	DefaultApiVersion = "1.40"
	MinimalApiVersion = "1.24"
)

type HttpServer struct {
	http.Server
	router   *mux.Router
	done     chan struct{}
	listener net.Listener
}

var libpodRuntime *libpod.Runtime

// NewServer will create and configure a new API HTTP server
func NewServer(runtime *libpod.Runtime) (*HttpServer, error) {
	libpodRuntime = runtime

	listeners, err := activation.Listeners()
	if err != nil {
		return nil, errors.Wrap(err, "Cannot retrieve listeners")
	}
	if len(listeners) != 1 {
		return nil, errors.Wrapf(err, "unexpected number of socket activation (%d != 1)", len(listeners))
	}

	done := make(chan struct{})
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Build routing rules for requests, based on URL path
	router := mux.NewRouter()
	for _, fn := range []func(*mux.Router) error{
		registerAuthHandlers,
		registerContainersHandlers,
		registerDistributionHandlers,
		registerImagesHandlers,
		registerInfoHandlers,
		registerMonitorHandlers,
		registerPingHandlers,
		registerPluginsHandlers,
		registerPodsHandlers,
		registerSwarmHandlers,
		registerSystemHandlers,
		registerVersionHandlers,
	} {
		fn(router)
	}
	registerNotFoundHandlers(router) // Should always be called last!

	server := HttpServer{http.Server{}, router, done, listeners[0]}
	go func() {
		<-quit
		log.Debugf("HttpServer is shutting down")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.SetKeepAlivesEnabled(false)
		if err := server.Shutdown(ctx); err != nil {
			log.Panicf("cannot gracefully shut down the http server: %s", err)
		}
		close(done)
	}()

	return &server, nil
}

// Serve starts responding to HTTP requests
func (s *HttpServer) Serve() error {
	err := http.Serve(s.listener, s.router)
	if err != nil {
		return errors.Wrap(err, "Failed to start HttpServer")
	}
	<-s.done
	return nil
}

// Shutdown is a clean shutdown waiting on existing clients
func (s *HttpServer) Shutdown(ctx context.Context) error {
	<-s.done
	return s.Server.Shutdown(ctx)
}

// Close immediately stops responding to clients and exits
func (s *HttpServer) Close() error {
	return s.Server.Close()
}

// unversionedPath prepends the version parsing code
// any handler may override this default when registering URL(s)
func unversionedPath(p string) string {
	return "/v{version:[0-9.]*}" + p
}
