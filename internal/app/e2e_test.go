package app

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/chaptersix/temporal-ts-net/internal/tailscale"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
	"tailscale.com/tstest/integration"
	"tailscale.com/tstest/integration/testcontrol"
	"tailscale.com/types/logger"
)

const (
	temporalCLIVersion = "1.6.1" // Version without 'v' prefix for filename
	temporalCLITag     = "v1.6.1" // Git tag with 'v' prefix
	testTimeout        = 60 * time.Second
)

// TestE2E_TemporalWorkflow tests end-to-end workflow execution through the proxy
func TestE2E_TemporalWorkflow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	// 1. Download/locate temporal CLI binary
	temporalBin := ensureTemporalCLI(t)

	// 2. Start Temporal dev server
	temporalAddr := startTemporalDevServer(t, ctx, temporalBin)

	// 3. Start testcontrol for tailnet
	controlURL := startTestControl(t)

	// 4. Start our proxy (connects temporal to tailnet)
	proxyHostname := "temporal-e2e-test"
	proxy := startProxyServer(t, ctx, controlURL, temporalAddr, proxyHostname, 7233)
	defer proxy.Stop()

	// 5. Create tsnet client node
	clientNode := startTsnetClient(t, ctx, controlURL, "e2e-client")
	defer clientNode.Close()

	// 6. Create Temporal SDK client connecting through proxy
	temporalClient := createTemporalClient(t, ctx, clientNode, proxyHostname, 7233)
	defer temporalClient.Close()

	// 7. Start worker with workflow/activity
	taskQueue := "e2e-test-queue"
	w := startWorker(t, temporalClient, taskQueue)
	defer w.Stop()

	// 8. Execute workflow
	workflowID := "e2e-test-workflow"
	workflowRun, err := temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: taskQueue,
	}, SimpleWorkflow, "test-input")
	require.NoError(t, err)

	// 9. Wait for workflow completion
	var result string
	err = workflowRun.Get(ctx, &result)
	require.NoError(t, err)
	require.Equal(t, "echo: test-input", result)

	t.Logf("✓ Workflow completed successfully: %s", result)
}

// SimpleWorkflow is a test workflow that echoes input
func SimpleWorkflow(ctx workflow.Context, input string) (string, error) {
	return fmt.Sprintf("echo: %s", input), nil
}

// ensureTemporalCLI downloads or locates the temporal CLI binary
func ensureTemporalCLI(t *testing.T) string {
	t.Helper()

	// Check if temporal is already in PATH
	if path, err := exec.LookPath("temporal"); err == nil {
		t.Logf("Using temporal CLI from PATH: %s", path)
		return path
	}

	// Download to temp directory
	cacheDir := t.TempDir()
	binName := "temporal"
	if runtime.GOOS == "windows" {
		binName = "temporal.exe"
	}
	binPath := filepath.Join(cacheDir, binName)

	// Check if already downloaded
	if _, err := os.Stat(binPath); err == nil {
		return binPath
	}

	// Download from GitHub releases
	t.Logf("Downloading temporal CLI %s...", temporalCLITag)

	goos := runtime.GOOS
	goarch := runtime.GOARCH
	// Use architecture names as-is (amd64, arm64)

	var archiveName string
	if goos == "windows" {
		archiveName = fmt.Sprintf("temporal_cli_%s_windows_%s.zip", temporalCLIVersion, goarch)
	} else if goos == "darwin" {
		archiveName = fmt.Sprintf("temporal_cli_%s_darwin_%s.tar.gz", temporalCLIVersion, goarch)
	} else {
		archiveName = fmt.Sprintf("temporal_cli_%s_linux_%s.tar.gz", temporalCLIVersion, goarch)
	}

	downloadURL := fmt.Sprintf("https://github.com/temporalio/cli/releases/download/%s/%s", temporalCLITag, archiveName)

	// Download archive
	archivePath := filepath.Join(cacheDir, archiveName)
	downloadFile(t, downloadURL, archivePath)

	// Extract binary
	if runtime.GOOS == "windows" {
		extractZip(t, archivePath, cacheDir)
	} else {
		extractTarGz(t, archivePath, cacheDir)
	}

	// Make executable
	if runtime.GOOS != "windows" {
		err := os.Chmod(binPath, 0755)
		require.NoError(t, err)
	}

	require.FileExists(t, binPath)
	t.Logf("Downloaded temporal CLI to: %s", binPath)
	return binPath
}

// downloadFile downloads a file from URL to destination
func downloadFile(t *testing.T, url, dest string) {
	t.Helper()

	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	out, err := os.Create(dest)
	require.NoError(t, err)
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	require.NoError(t, err)
}

// extractTarGz extracts a tar.gz archive
func extractTarGz(t *testing.T, archive, dest string) {
	t.Helper()
	cmd := exec.Command("tar", "-xzf", archive, "-C", dest)
	err := cmd.Run()
	require.NoError(t, err)
}

// extractZip extracts a zip archive
func extractZip(t *testing.T, archive, dest string) {
	t.Helper()
	cmd := exec.Command("unzip", "-q", archive, "-d", dest)
	err := cmd.Run()
	require.NoError(t, err)
}

// startTemporalDevServer starts temporal server start-dev
func startTemporalDevServer(t *testing.T, ctx context.Context, temporalBin string) string {
	t.Helper()

	// Use random available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	dbPath := filepath.Join(t.TempDir(), "temporal.db")

	cmd := exec.CommandContext(ctx, temporalBin, "server", "start-dev",
		"--port", fmt.Sprintf("%d", port),
		"--db-filename", dbPath,
		"--headless",
	)

	// Discard output to avoid buffering issues
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard

	err = cmd.Start()
	require.NoError(t, err)

	// Wait for server to be ready
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	waitForTCPReady(t, addr, 30*time.Second)

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Signal(os.Interrupt)
			time.Sleep(2 * time.Second)
			_ = cmd.Process.Kill()
		}
	})

	t.Logf("Temporal dev server started on %s", addr)
	return addr
}

// waitForTCPReady waits for a TCP port to be ready
func waitForTCPReady(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("TCP port %s not ready after %s", addr, timeout)
}

// startTestControl starts a testcontrol server with DERP and STUN
func startTestControl(t *testing.T) string {
	t.Helper()

	// Start DERP and STUN servers
	derpMap := integration.RunDERPAndSTUN(t, logger.Discard, "127.0.0.1")

	// Start control server
	control := &testcontrol.Server{
		DERPMap: derpMap,
		DNSConfig: &tailcfg.DNSConfig{
			Proxied: true,
		},
		MagicDNSDomain: "tail-scale.ts.net",
	}
	control.HTTPTestServer = httptest.NewUnstartedServer(control)
	control.HTTPTestServer.Start()
	t.Cleanup(control.HTTPTestServer.Close)

	return control.HTTPTestServer.URL
}

// startProxyServer starts our tailscale proxy
func startProxyServer(t *testing.T, ctx context.Context, controlURL, backendAddr, hostname string, port int) *tailscale.Server {
	t.Helper()

	proxy, err := tailscale.Start(ctx, tailscale.Options{
		Hostname:     hostname,
		FrontendAddr: backendAddr,
		FrontendPort: port,
		StateDir:     t.TempDir(),
		ControlURL:   controlURL,
	})
	require.NoError(t, err)

	// Wait for proxy to be ready
	waitForTsnetReady(t, proxy.GetServer(), 10*time.Second)

	t.Logf("Proxy started: %s:%d -> %s", hostname, port, backendAddr)
	return proxy
}

// startTsnetClient starts a tsnet client node
func startTsnetClient(t *testing.T, ctx context.Context, controlURL, hostname string) *tsnet.Server {
	t.Helper()

	client := &tsnet.Server{
		Hostname:   hostname,
		Dir:        t.TempDir(),
		ControlURL: controlURL,
		Ephemeral:  true,
	}

	// Wait for client to be ready
	waitForTsnetReady(t, client, 10*time.Second)

	return client
}

// waitForTsnetReady waits for a tsnet server to be running
func waitForTsnetReady(t *testing.T, srv *tsnet.Server, timeout time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	_, err := srv.Up(ctx)
	require.NoError(t, err, "failed to bring up tsnet server")
}

// createTemporalClient creates a Temporal SDK client using tsnet dialer
func createTemporalClient(t *testing.T, ctx context.Context, tsnetClient *tsnet.Server, hostname string, port int) client.Client {
	t.Helper()

	targetAddr := fmt.Sprintf("%s:%d", hostname, port)

	// Create custom dialer using tsnet - intercept all dial attempts
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		t.Logf("Intercepting dial to %s, connecting via tsnet to %s", addr, targetAddr)
		return tsnetClient.Dial(ctx, "tcp", targetAddr)
	}

	temporalClient, err := client.Dial(client.Options{
		HostPort: "localhost:7233", // Will be intercepted by custom dialer
		ConnectionOptions: client.ConnectionOptions{
			DialOptions: []grpc.DialOption{
				grpc.WithBlock(),
				grpc.WithContextDialer(dialer),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			},
		},
	})
	require.NoError(t, err)

	return temporalClient
}

// startWorker starts a Temporal worker
func startWorker(t *testing.T, c client.Client, taskQueue string) worker.Worker {
	t.Helper()

	w := worker.New(c, taskQueue, worker.Options{})
	w.RegisterWorkflow(SimpleWorkflow)

	err := w.Start()
	require.NoError(t, err)

	return w
}
