// Command flixbus-mcp serves FlixBus search tools over the Model Context
// Protocol, on stdio or streamable HTTP.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/adambenhassen/flixbus-mcp/internal/config"
	"github.com/adambenhassen/flixbus-mcp/internal/flixbus"
	"github.com/adambenhassen/flixbus-mcp/internal/tools"
)

// version is stamped by the release build via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	transport := flag.String("transport", "http", "transport: stdio or http")
	addr := flag.String("addr", "127.0.0.1:8748", "listen address for the http transport (loopback by default; pass :8748 to expose)")
	flag.Parse()

	cfg, err := config.FromEnv()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := flixbus.New(cfg.BaseURL, cfg.APIKey, cfg.Timeout)
	server := mcp.NewServer(&mcp.Implementation{Name: "flixbus-mcp", Version: version}, nil)
	tools.Register(server, &tools.Deps{Client: client, Cfg: cfg})

	switch *transport {
	case "stdio":
		err := server.Run(ctx, &mcp.StdioTransport{})
		if errors.Is(err, context.Canceled) && ctx.Err() != nil {
			return nil // SIGINT/SIGTERM is a normal shutdown, not a failure
		}
		return err
	case "http":
		handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, nil)
		srv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
		shutdownErr := make(chan error, 1)
		go func() {
			<-ctx.Done()
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			shutdownErr <- srv.Shutdown(shutdownCtx)
		}()
		log.Printf("flixbus-mcp %s listening on %s", version, *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		if ctx.Err() != nil {
			if err := <-shutdownErr; err != nil {
				return fmt.Errorf("shutdown: %w", err)
			}
		}
		return nil
	default:
		return fmt.Errorf("unknown transport %q (want stdio or http)", *transport)
	}
}
