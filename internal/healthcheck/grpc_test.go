package healthcheck

import (
	"context"
	"log/slog"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"

	"watchdawg/internal/models"
)

const bufSize = 1024 * 1024

// startHealthServer starts an in-process gRPC server using bufconn.
// The returned health.Server can be used to change the status during tests.
// Call the cleanup function when done.
func startHealthServer(t *testing.T) (*health.Server, func(target string, _ bool, _ *bool) (*grpc.ClientConn, error)) {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			// Server stopped — normal on test cleanup.
		}
	}()

	t.Cleanup(func() {
		grpcServer.Stop()
		lis.Close()
	})

	dial := func(_ string, _ bool, _ *bool) (*grpc.ClientConn, error) {
		return grpc.NewClient(
			"passthrough://bufnet",
			grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
				return lis.DialContext(ctx)
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
	}

	return healthSrv, dial
}

func newTestGRPCCheck(service string) *models.HealthCheck {
	return &models.HealthCheck{
		Name:    "test-grpc",
		Retries: 0,
		Timeout: 5 * time.Second,
		GRPC: &models.GRPCCheckConfig{
			Target:    "passthrough://bufnet",
			PlainText: true,
			Service:   service,
		},
	}
}

func TestGRPCChecker_ServerLevel_Serving(t *testing.T) {
	healthSrv, dial := startHealthServer(t)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

	checker := &GRPCChecker{logger: slog.Default(), recorder: NoopMetricsRecorder{}, dial: dial}
	result := checker.Execute(context.Background(), newTestGRPCCheck(""))

	if !result.Healthy {
		t.Errorf("expected healthy, got unhealthy: %s", result.Message)
	}
	if result.GRPCResult == nil || result.GRPCResult.HealthStatus != "SERVING" {
		t.Errorf("expected HealthStatus=SERVING, got %v", result.GRPCResult)
	}
	if result.Attempt != 1 {
		t.Errorf("expected attempt=1, got %d", result.Attempt)
	}
}

func TestGRPCChecker_ServerLevel_NotServing(t *testing.T) {
	healthSrv, dial := startHealthServer(t)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	checker := &GRPCChecker{logger: slog.Default(), recorder: NoopMetricsRecorder{}, dial: dial}
	result := checker.Execute(context.Background(), newTestGRPCCheck(""))

	if result.Healthy {
		t.Errorf("expected unhealthy, got healthy")
	}
	if result.GRPCResult == nil || result.GRPCResult.HealthStatus != "NOT_SERVING" {
		t.Errorf("expected HealthStatus=NOT_SERVING, got %v", result.GRPCResult)
	}
}

func TestGRPCChecker_NamedService_Serving(t *testing.T) {
	healthSrv, dial := startHealthServer(t)
	healthSrv.SetServingStatus("my.package.MyService", grpc_health_v1.HealthCheckResponse_SERVING)

	checker := &GRPCChecker{logger: slog.Default(), recorder: NoopMetricsRecorder{}, dial: dial}
	result := checker.Execute(context.Background(), newTestGRPCCheck("my.package.MyService"))

	if !result.Healthy {
		t.Errorf("expected healthy, got unhealthy: %s", result.Message)
	}
}

func TestGRPCChecker_NamedService_Unknown(t *testing.T) {
	_, dial := startHealthServer(t)
	// Service not registered → grpc health returns SERVICE_UNKNOWN

	checker := &GRPCChecker{logger: slog.Default(), recorder: NoopMetricsRecorder{}, dial: dial}
	result := checker.Execute(context.Background(), newTestGRPCCheck("not.registered.Service"))

	if result.Healthy {
		t.Errorf("expected unhealthy for SERVICE_UNKNOWN status")
	}
}

func TestGRPCChecker_ConnectionFailure(t *testing.T) {
	dial := func(_ string, _ bool, _ *bool) (*grpc.ClientConn, error) {
		// Return a connection to a non-existent address; the RPC call will fail.
		return grpc.NewClient("localhost:1", grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	checker := &GRPCChecker{logger: slog.Default(), recorder: NoopMetricsRecorder{}, dial: dial}
	check := newTestGRPCCheck("")
	check.Timeout = 500 * time.Millisecond

	result := checker.Execute(context.Background(), check)

	if result.Healthy {
		t.Errorf("expected unhealthy for connection failure")
	}
	if result.Error == "" {
		t.Errorf("expected non-empty Error field on connection failure")
	}
}

func TestGRPCChecker_RetrySucceeds(t *testing.T) {
	healthSrv, dial := startHealthServer(t)
	// Start NOT_SERVING; flip to SERVING after a short delay so the retry succeeds.
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	go func() {
		time.Sleep(1200 * time.Millisecond) // just after the 1s retry sleep
		healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	}()

	checker := &GRPCChecker{logger: slog.Default(), recorder: NoopMetricsRecorder{}, dial: dial}
	check := newTestGRPCCheck("")
	check.Retries = 2

	result := checker.Execute(context.Background(), check)

	if !result.Healthy {
		t.Errorf("expected healthy after retry, got: %s", result.Message)
	}
	if result.Attempt < 2 {
		t.Errorf("expected at least 2 attempts, got %d", result.Attempt)
	}
}

func TestGRPCChecker_MessageContainsServiceName(t *testing.T) {
	healthSrv, dial := startHealthServer(t)
	healthSrv.SetServingStatus("svc.MyService", grpc_health_v1.HealthCheckResponse_SERVING)

	checker := &GRPCChecker{logger: slog.Default(), recorder: NoopMetricsRecorder{}, dial: dial}
	result := checker.Execute(context.Background(), newTestGRPCCheck("svc.MyService"))

	if !result.Healthy {
		t.Fatalf("expected healthy")
	}
	// Message should include the service name so operators know which service was checked.
	const serviceName = "svc.MyService"
	if len(result.Message) == 0 {
		t.Errorf("expected non-empty message")
	}
	for _, substr := range []string{serviceName, "SERVING"} {
		found := false
		for i := 0; i <= len(result.Message)-len(substr); i++ {
			if result.Message[i:i+len(substr)] == substr {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected message to contain %q, got: %s", substr, result.Message)
		}
	}
}
