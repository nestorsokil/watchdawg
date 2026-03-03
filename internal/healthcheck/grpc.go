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
	logger *slog.Logger
	// dial is injectable so tests can replace the real dialer with a bufconn-backed one.
	dial func(target string, plainText bool, verifyTLS *bool) (*grpc.ClientConn, error)
}

func NewGRPCChecker(logger *slog.Logger) *GRPCChecker {
	return &GRPCChecker{
		logger: logger,
		dial:   dialGRPC,
	}
}

func (g *GRPCChecker) Execute(ctx context.Context, check *models.HealthCheck) *models.CheckResult {
	startTime := time.Now()

	var lastResult *models.CheckResult
	maxAttempts := check.Retries + 1

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result := g.executeOnce(ctx, check, attempt)
		lastResult = result

		if result.Healthy {
			result.Duration = time.Since(startTime).Milliseconds()
			return result
		}

		if attempt < maxAttempts {
			time.Sleep(time.Second)
		}
	}

	lastResult.Duration = time.Since(startTime).Milliseconds()
	return lastResult
}

func (g *GRPCChecker) executeOnce(ctx context.Context, check *models.HealthCheck, attempt int) *models.CheckResult {
	result := &models.CheckResult{
		CheckName: check.Name,
		Timestamp: time.Now(),
		Attempt:   attempt,
	}

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
