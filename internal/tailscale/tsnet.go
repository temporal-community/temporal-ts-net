package tailscale

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"

	"tailscale.com/tsnet"
)

type Options struct {
	Hostname     string
	AuthKey      string
	StateDir     string
	FrontendAddr string
	UIAddr       string
	FrontendPort int
	UIPort       int
	Logger       *slog.Logger
}

type Server struct {
	Hostname  string
	server    *tsnet.Server
	listeners []net.Listener
	logger    *slog.Logger
	cancel    context.CancelFunc
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

type halfCloser interface {
	CloseWrite() error
}

func Start(parent context.Context, opts Options) (*Server, error) {
	ctx, cancel := context.WithCancel(parent)

	stateDir := opts.StateDir
	if stateDir == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("determine config directory: %w", err)
		}
		stateDir = filepath.Join(configDir, "tsnet-temporal-ts-net")
	}

	tsSrv := &tsnet.Server{
		Hostname: opts.Hostname,
		AuthKey:  opts.AuthKey,
		Dir:      stateDir,
	}

	if err := tsSrv.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start tsnet: %w", err)
	}

	s := &Server{
		Hostname: opts.Hostname,
		server:   tsSrv,
		logger:   opts.Logger,
		cancel:   cancel,
	}

	frontendLn, err := tsSrv.Listen("tcp", fmt.Sprintf(":%d", opts.FrontendPort))
	if err != nil {
		s.Stop()
		return nil, fmt.Errorf("listen tsnet gRPC port %d: %w", opts.FrontendPort, err)
	}
	s.listeners = append(s.listeners, frontendLn)
	go acceptLoop(frontendLn, opts.FrontendAddr, s.logger, &s.wg)

	if opts.UIAddr != "" && opts.UIPort > 0 {
		uiLn, err := tsSrv.Listen("tcp", fmt.Sprintf(":%d", opts.UIPort))
		if err != nil {
			s.Stop()
			return nil, fmt.Errorf("listen tsnet UI port %d: %w", opts.UIPort, err)
		}
		s.listeners = append(s.listeners, uiLn)
		go acceptLoop(uiLn, opts.UIAddr, s.logger, &s.wg)
	}

	if opts.Logger != nil {
		opts.Logger.InfoContext(ctx, "tsnet node started", "hostname", opts.Hostname)
	}

	return s, nil
}

func (s *Server) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}

		for _, ln := range s.listeners {
			if err := ln.Close(); err != nil && s.logger != nil {
				s.logger.Warn("failed to close tsnet listener", "error", err)
			}
		}

		s.wg.Wait()

		if s.server != nil {
			if err := s.server.Close(); err != nil && s.logger != nil {
				s.logger.Warn("failed to close tsnet server", "error", err)
			}
		}
	})
}

func acceptLoop(ln net.Listener, targetAddr string, logger *slog.Logger, wg *sync.WaitGroup) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if logger != nil {
				logger.Warn("accept error, retrying", "error", err)
			}
			continue
		}
		if wg != nil {
			wg.Add(1)
		}
		go proxy(conn, targetAddr, logger, wg)
	}
}

func proxy(src net.Conn, dstAddr string, logger *slog.Logger, parentWg *sync.WaitGroup) {
	if parentWg != nil {
		defer parentWg.Done()
	}

	dst, err := net.Dial("tcp", dstAddr)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to dial proxy destination", "addr", dstAddr, "error", err)
		}
		src.Close()
		return
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, err := io.Copy(dst, src)
		if err != nil && !isClosedErr(err) && logger != nil {
			logger.Debug("proxy copy src->dst ended", "error", err)
		}
		if hc, ok := dst.(halfCloser); ok {
			_ = hc.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}()

	go func() {
		defer wg.Done()
		_, err := io.Copy(src, dst)
		if err != nil && !isClosedErr(err) && logger != nil {
			logger.Debug("proxy copy dst->src ended", "error", err)
		}
		if hc, ok := src.(halfCloser); ok {
			_ = hc.CloseWrite()
		} else {
			_ = src.Close()
		}
	}()

	wg.Wait()
	_ = src.Close()
	_ = dst.Close()
}

func isClosedErr(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe)
}
