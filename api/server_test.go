package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestRunWithoutInit(t *testing.T) {
	server := &Server{}
	if err := server.Run(); err == nil {
		t.Fatal("expected Run to fail before Init, got nil")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("unable to reserve a port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("unable to release reserved port: %v", err)
	}
	return port
}

func TestServerRunServesAndShutsDownOnSignal(t *testing.T) {
	handler := newTestRoutes(t)
	port := freePort(t)

	server := &Server{}
	server.Init(port, handler)
	runErr := make(chan error, 1)
	go func() {
		runErr <- server.Run()
	}()

	// Wait until the server answers before signalling; t.Context cancels
	// the request if the test fails or times out.
	deadline := time.Now().Add(5 * time.Second)
	for {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/api/v1/", port), nil)
		if err != nil {
			t.Fatalf("unable to build probe request: %v", err)
		}
		resp, err := http.DefaultClient.Do(request)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("server did not start serving requests")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("unable to send SIGTERM: %v", err)
	}
	select {
	case err := <-runErr:
		if err != nil {
			t.Fatalf("expected clean shutdown, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down after SIGTERM")
	}
}

// TestServerRunReturnsListenError covers the case where the port cannot be
// bound: the listen error must propagate out of Run.
func TestServerRunReturnsListenError(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("unable to reserve a port: %v", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port

	server := &Server{}
	server.Init(port, http.NewServeMux())
	err = server.Run()
	if err == nil {
		t.Fatal("expected listen error, got nil")
	}
	if !strings.Contains(err.Error(), "address already in use") {
		t.Errorf("expected address-in-use error, got %v", err)
	}
}
