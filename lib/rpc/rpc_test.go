package rpc

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	taskerpb "github.com/wolves-fc/tasker/gen/proto/tasker"
	"github.com/wolves-fc/tasker/lib/tls"
)

type mockService struct {
	taskerpb.UnimplementedTaskerServiceServer
	mu struct {
		sync.Mutex
		identity Identity
	}
}

// GetJob is the only method implemented for testing to capture the caller's identity.
func (m *mockService) GetJob(ctx context.Context, req *taskerpb.GetJobRequest) (*taskerpb.GetJobResponse, error) {
	id, err := IdentityFromContext(ctx)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	m.mu.identity = id
	m.mu.Unlock()

	return &taskerpb.GetJobResponse{}, nil
}

// getAddr finds an available TCP port to listen on.
//
// Serve does the actual binding so that is why this just gets an address.
func getAddr(t *testing.T) string {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	defer listener.Close()

	return listener.Addr().String()
}

func generateCerts(t *testing.T, dir string) {
	t.Helper()

	if err := tls.GenerateCA(dir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	if err := tls.GenerateServer(dir, "wolfpack1", []string{"127.0.0.1"}); err != nil {
		t.Fatalf("GenerateServer: %v", err)
	}

	if err := tls.GenerateClient(dir, "wolf", tls.RoleAdmin); err != nil {
		t.Fatalf("GenerateClient (admin): %v", err)
	}

	if err := tls.GenerateClient(dir, "wolfjr", tls.RoleUser); err != nil {
		t.Fatalf("GenerateClient (user): %v", err)
	}
}

func startServer(t *testing.T, certDir, addr string) *mockService {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	svc := &mockService{}

	errChan := make(chan error, 1)
	go func() {
		errChan <- Serve(ctx, svc, certDir, "wolfpack1", addr)
	}()

	waitForServer(t, addr)

	t.Cleanup(func() {
		cancel()
		if err := <-errChan; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})

	return svc
}

func waitForServer(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("server did not start")
}

func TestServe_RoleAccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	generateCerts(t, dir)
	addr := getAddr(t)
	svc := startServer(t, dir, addr)

	for _, tc := range []struct {
		user string
		role tls.Role
	}{
		{"wolf", tls.RoleAdmin},
		{"wolfjr", tls.RoleUser},
	} {
		t.Run(string(tc.role), func(t *testing.T) {
			clt, err := Dial(dir, tc.user, addr)
			if err != nil {
				t.Fatalf("Dial: %v", err)
			}

			defer clt.Close()

			if _, err := clt.Tasker.GetJob(context.Background(), &taskerpb.GetJobRequest{}); err != nil {
				t.Fatalf("GetJob: %v", err)
			}

			svc.mu.Lock()
			got := svc.mu.identity
			svc.mu.Unlock()

			if got.Name != tc.user {
				t.Errorf("Name (got=%q, want=%q)", got.Name, tc.user)
			}

			if got.Role != tc.role {
				t.Errorf("Role (got=%q, want=%q)", got.Role, tc.role)
			}
		})
	}
}

func TestServe_Shutdown(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	generateCerts(t, dir)
	addr := getAddr(t)

	ctx, cancel := context.WithCancel(context.Background())
	svc := &mockService{}

	// Start serving and send errors to a channel.
	errChan := make(chan error, 1)
	go func() {
		errChan <- Serve(ctx, svc, dir, "wolfpack1", addr)
	}()

	waitForServer(t, addr)
	cancel()

	select {
	case err := <-errChan:
		if err != nil {
			t.Fatalf("Serve after shutdown (got=%v, want=nil)", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}
}

func TestClient_PeerIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	generateCerts(t, dir)
	addr := getAddr(t)
	startServer(t, dir, addr)

	clt, err := Dial(dir, "wolf", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer clt.Close()

	// Trigger the TLS handshake so PeerIdentity gets populated.
	if _, err := clt.Tasker.GetJob(context.Background(), &taskerpb.GetJobRequest{}); err != nil {
		t.Fatalf("GetJob: %v", err)
	}

	got := clt.PeerIdentity()
	if got.Name != "wolfpack1" {
		t.Errorf("Name (got=%q, want=%q)", got.Name, "wolfpack1")
	}

	if got.Role != tls.RoleServer {
		t.Errorf("Role (got=%q, want=%q)", got.Role, tls.RoleServer)
	}
}

func TestClient_WrongCA(t *testing.T) {
	t.Parallel()

	serverDir := t.TempDir()
	generateCerts(t, serverDir)
	addr := getAddr(t)
	startServer(t, serverDir, addr)

	// Generate a separate CA and client cert that the server doesn't trust.
	clientDir := t.TempDir()
	if err := tls.GenerateCA(clientDir); err != nil {
		t.Fatalf("GenerateCA: %v", err)
	}

	if err := tls.GenerateClient(clientDir, "untrusted", tls.RoleAdmin); err != nil {
		t.Fatalf("GenerateClient: %v", err)
	}

	clt, err := Dial(clientDir, "untrusted", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	defer clt.Close()

	_, err = clt.Tasker.GetJob(context.Background(), &taskerpb.GetJobRequest{})
	if err == nil {
		t.Fatal("GetJob with wrong CA (got=nil, want=error)")
	}
}


