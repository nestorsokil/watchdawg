"""gRPC health stub for integration tests.

Runs an in-process gRPC server implementing grpc.health.v1.Health.
Tests can flip the serving status per service name via GRPCStub methods.

State is module-level so the session fixture owns the lifetime.
"""
import concurrent.futures
import grpc
from grpc_health.v1 import health, health_pb2, health_pb2_grpc

PORT = 50051
NAMED_SERVICE = "integration.TestService"

_health_servicer = None
_server = None


class GRPCStub:
    """Test-facing wrapper that controls the in-process gRPC server state."""

    def set_serving(self, service=""):
        _health_servicer.set(service, health_pb2.HealthCheckResponse.SERVING)

    def set_not_serving(self, service=""):
        _health_servicer.set(service, health_pb2.HealthCheckResponse.NOT_SERVING)


def reset():
    """Restore all services to SERVING between tests."""
    _health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    _health_servicer.set(NAMED_SERVICE, health_pb2.HealthCheckResponse.SERVING)


def main():
    global _health_servicer, _server

    _health_servicer = health.HealthServicer()
    _health_servicer.set("", health_pb2.HealthCheckResponse.SERVING)
    _health_servicer.set(NAMED_SERVICE, health_pb2.HealthCheckResponse.SERVING)

    _server = grpc.server(concurrent.futures.ThreadPoolExecutor(max_workers=4))
    health_pb2_grpc.add_HealthServicer_to_server(_health_servicer, _server)
    _server.add_insecure_port(f"[::]:{PORT}")
    _server.start()
    _server.wait_for_termination()
