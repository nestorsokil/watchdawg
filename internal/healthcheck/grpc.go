package healthcheck

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"

	"watchdawg/internal/models"
)

type GRPCChecker struct {
	NoOpInitializer
	logger   *slog.Logger
	recorder MetricsRecorder
	// dial is injectable so tests can replace the real dialer with a bufconn-backed one.
	dial func(target string, plainText bool, verifyTLS *bool) (*grpc.ClientConn, error)
}

func NewGRPCChecker(logger *slog.Logger, recorder MetricsRecorder) *GRPCChecker {
	return &GRPCChecker{
		logger:   logger,
		recorder: recorder,
		dial:     dialGRPC,
	}
}

func (k *GRPCChecker) IsMatching(check *models.HealthCheck) bool { return check.GRPC != nil }

func (g *GRPCChecker) Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult {
	return executeWithRetry(ctx, check, g.executeOnce)
}

func (g *GRPCChecker) executeOnce(ctx context.Context, check *models.HealthCheck, attempt int) *models.CheckResult {
	attemptStart := time.Now()
	result := &models.CheckResult{
		CheckName: check.Name,
		Timestamp: attemptStart,
		Attempt:   attempt,
	}
	defer func() {
		g.recorder.RecordCheckAttempt(check.Name, result.Healthy, time.Since(attemptStart).Seconds())
	}()

	conn, err := g.dial(check.GRPC.Target, check.GRPC.PlainText, check.GRPC.VerifyTLS)
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("failed to connect to %s: %v", check.GRPC.Target, err)
		result.Message = result.Error
		return result
	}
	defer conn.Close()

	client := grpc_health_v1.NewHealthClient(conn)
	resp, err := client.Check(ctx, &grpc_health_v1.HealthCheckRequest{
		Service: check.GRPC.Service,
	})
	if err != nil {
		result.Healthy = false
		result.Error = fmt.Sprintf("health check RPC failed: %v", err)
		result.Message = result.Error
		return result
	}

	result.GRPCResult = &models.GRPCResult{
		HealthStatus: resp.Status.String(),
	}

	serviceName := check.GRPC.Service
	if serviceName == "" {
		serviceName = "(server-level)"
	}

	if resp.Status == grpc_health_v1.HealthCheckResponse_SERVING {
		result.Healthy = true
		result.Message = fmt.Sprintf("gRPC check passed: %s is SERVING", serviceName)
	} else {
		result.Healthy = false
		result.Message = fmt.Sprintf("gRPC check failed: %s status is %s", serviceName, resp.Status)
	}

	return result
}

// dialGRPC creates a gRPC client connection to target.
// When plainText is true the connection is unencrypted; otherwise TLS is used,
// with certificate verification skipped when verifyTLS is explicitly false.
func dialGRPC(target string, plainText bool, verifyTLS *bool) (*grpc.ClientConn, error) {
	if plainText {
		return grpc.NewClient(target, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}

	tlsCfg := &tls.Config{}
	if verifyTLS != nil && !*verifyTLS {
		tlsCfg.InsecureSkipVerify = true
	}
	return grpc.NewClient(target, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
}
