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
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
	"tailscale.com/tsnet"
)

type Options struct {
	Hostname            string
	AuthKey             string
	StateDir            string
	ControlURL          string // For testing with testcontrol; defaults to production if empty
	FrontendAddr        string
	UIAddr              string
	FrontendPort        int
	UIPort              int
	Logger              *slog.Logger
	MaxConnections      int
	ConnectionRateLimit float64
	DialTimeout         time.Duration
	IdleTimeout         time.Duration
}

type Server struct {
	Hostname          string
	server            *tsnet.Server
	listeners         []net.Listener
	logger            *slog.Logger
	cancel            context.CancelFunc
	stopOnce          sync.Once
	wg                sync.WaitGroup
	activeConnections atomic.Int32
	maxConnections    int
	rateLimiter       *rate.Limiter
	dialTimeout       time.Duration
	idleTimeout       time.Duration
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
		Hostname:   opts.Hostname,
		AuthKey:    opts.AuthKey,
		Dir:        stateDir,
		ControlURL: opts.ControlURL,
	}

	if err := tsSrv.Start(); err != nil {
		cancel()
		return nil, fmt.Errorf("start tsnet: %w", err)
	}

	s := &Server{
		Hostname:       opts.Hostname,
		server:         tsSrv,
		logger:         opts.Logger,
		cancel:         cancel,
		maxConnections: opts.MaxConnections,
		dialTimeout:    opts.DialTimeout,
		idleTimeout:    opts.IdleTimeout,
	}

	if opts.ConnectionRateLimit > 0 {
		s.rateLimiter = rate.NewLimiter(rate.Limit(opts.ConnectionRateLimit), int(opts.ConnectionRateLimit))
	}

	frontendLn, err := tsSrv.Listen("tcp", fmt.Sprintf(":%d", opts.FrontendPort))
	if err != nil {
		s.Stop()
		return nil, fmt.Errorf("listen tsnet gRPC port %d: %w", opts.FrontendPort, err)
	}
	s.listeners = append(s.listeners, frontendLn)
	go acceptLoop(ctx, frontendLn, opts.FrontendAddr, s)

	if opts.UIAddr != "" && opts.UIPort > 0 {
		uiLn, err := tsSrv.Listen("tcp", fmt.Sprintf(":%d", opts.UIPort))
		if err != nil {
			s.Stop()
			return nil, fmt.Errorf("listen tsnet UI port %d: %w", opts.UIPort, err)
		}
		s.listeners = append(s.listeners, uiLn)
		go acceptLoop(ctx, uiLn, opts.UIAddr, s)
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

func acceptLoop(ctx context.Context, ln net.Listener, targetAddr string, srv *Server) {
	for {
		// Check rate limit
		if srv.rateLimiter != nil {
			if err := srv.rateLimiter.Wait(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				if srv.logger != nil {
					srv.logger.Warn("rate limiter error", "error", err)
				}
				continue
			}
		}

		// Check connection limit
		current := srv.activeConnections.Load()
		if srv.maxConnections > 0 && int(current) >= srv.maxConnections {
			if srv.logger != nil {
				srv.logger.Warn("connection limit reached, waiting...",
					"current", current, "max", srv.maxConnections)
			}
			time.Sleep(100 * time.Millisecond)
			continue
		}

		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if srv.logger != nil {
				srv.logger.Warn("accept error, retrying", "error", err)
			}
			continue
		}

		// Increment connection count
		srv.activeConnections.Add(1)

		srv.wg.Add(1)
		go proxy(conn, targetAddr, srv)
	}
}

func proxy(src net.Conn, dstAddr string, srv *Server) {
	defer srv.activeConnections.Add(-1)
	defer srv.wg.Done()

	// Set idle timeout on source connection
	if srv.idleTimeout > 0 {
		if err := src.SetDeadline(time.Now().Add(srv.idleTimeout)); err != nil {
			if srv.logger != nil {
				srv.logger.Warn("failed to set source deadline", "error", err)
			}
		}
	}

	// Dial with timeout
	dialer := &net.Dialer{
		Timeout: srv.dialTimeout,
	}
	dst, err := dialer.Dial("tcp", dstAddr)
	if err != nil {
		if srv.logger != nil {
			srv.logger.Warn("failed to dial proxy destination", "addr", dstAddr, "error", err)
		}
		src.Close()
		return
	}

	// Set idle timeout on destination connection
	if srv.idleTimeout > 0 {
		if err := dst.SetDeadline(time.Now().Add(srv.idleTimeout)); err != nil {
			if srv.logger != nil {
				srv.logger.Warn("failed to set destination deadline", "error", err)
			}
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Copy src -> dst with deadline refresh
	go func() {
		defer wg.Done()
		_, err := copyWithDeadlineRefresh(dst, src, srv.idleTimeout)
		if err != nil && !isClosedErr(err) && srv.logger != nil {
			srv.logger.Debug("proxy copy src->dst ended", "error", err)
		}
		if hc, ok := dst.(halfCloser); ok {
			_ = hc.CloseWrite()
		} else {
			_ = dst.Close()
		}
	}()

	// Copy dst -> src with deadline refresh
	go func() {
		defer wg.Done()
		_, err := copyWithDeadlineRefresh(src, dst, srv.idleTimeout)
		if err != nil && !isClosedErr(err) && srv.logger != nil {
			srv.logger.Debug("proxy copy dst->src ended", "error", err)
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

func copyWithDeadlineRefresh(dst net.Conn, src net.Conn, timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		return io.Copy(dst, src)
	}

	buf := make([]byte, 32*1024)
	var written int64

	for {
		// Refresh deadline before each read
		if err := src.SetDeadline(time.Now().Add(timeout)); err != nil {
			return written, err
		}

		nr, er := src.Read(buf)
		if nr > 0 {
			// Refresh deadline before write
			if err := dst.SetDeadline(time.Now().Add(timeout)); err != nil {
				return written, err
			}

			nw, ew := dst.Write(buf[0:nr])
			if nw < 0 || nr < nw {
				nw = 0
				if ew == nil {
					ew = errors.New("invalid write result")
				}
			}
			written += int64(nw)
			if ew != nil {
				return written, ew
			}
			if nr != nw {
				return written, io.ErrShortWrite
			}
		}
		if er != nil {
			if er != io.EOF {
				return written, er
			}
			break
		}
	}
	return written, nil
}

func isClosedErr(err error) bool {
	return errors.Is(err, net.ErrClosed) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrClosedPipe)
}
