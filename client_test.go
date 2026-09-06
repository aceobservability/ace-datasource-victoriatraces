package victoriatraces

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aceobservability/ace-datasource-tempo/tracing"
)

func TestNew_requiresHTTPClient(t *testing.T) {
	t.Parallel()

	client, err := New("http://localhost:10428", nil)
	if err == nil {
		t.Fatal("expected error for nil http client")
	}
	if client != nil {
		t.Fatal("expected nil client when http client is missing")
	}
	if !strings.Contains(err.Error(), "http client is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_requiresURL(t *testing.T) {
	t.Parallel()

	client, err := New("", http.DefaultClient)
	if err == nil {
		t.Fatal("expected error for empty url")
	}
	if client != nil {
		t.Fatal("expected nil client when url is missing")
	}
}

func TestClient_ServicesFallback(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/select/jaeger/api/services":
			w.WriteHeader(http.StatusNotFound)
		case "/api/services":
			_, _ = w.Write([]byte(`["frontend","worker"]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	client, err := New(server.URL, server.Client())
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	services, err := client.Services(context.Background())
	if err != nil {
		t.Fatalf("Services returned error: %v", err)
	}

	if !slices.Equal(services, []string{"frontend", "worker"}) {
		t.Fatalf("expected services [frontend worker], got %#v", services)
	}
}

func TestQueryAndTestConnection_againstFixtureHTTP(t *testing.T) {
	t.Parallel()

	var sawHealth, sawTrace, sawJaegerServices bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/health":
			sawHealth = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		case strings.HasPrefix(r.URL.Path, "/select/jaeger/api/traces/"):
			sawTrace = true
			_, _ = w.Write([]byte(`{"data":[{"traceID":"trace-123","spans":[{"traceID":"trace-123","spanID":"root","operationName":"GET /","references":[],"startTime":1700000000000000,"duration":1000,"tags":[],"processID":"p1"}],"processes":{"p1":{"serviceName":"frontend"}}}]}`))
		case r.URL.Path == "/select/jaeger/api/traces":
			_, _ = w.Write([]byte(`{"data":[{"traceID":"trace-jaeger","spans":[{"traceID":"trace-jaeger","spanID":"root","operationName":"GET /","references":[],"startTime":1700000000000000,"duration":1000,"tags":[],"processID":"p1"}],"processes":{"p1":{"serviceName":"frontend"}}}]}`))
		case r.URL.Path == "/select/jaeger/api/services":
			sawJaegerServices = true
			_, _ = w.Write([]byte(`{"data":["frontend"]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if _, err := client.Query(ctx, "{}", time.Now().Add(-time.Hour), time.Now(), time.Minute, 0); err == nil {
		t.Fatal("expected Query to reject tracing datasources")
	}

	trace, err := client.GetTrace(ctx, "trace-123")
	if err != nil {
		t.Fatalf("GetTrace: %v", err)
	}
	if trace.TraceID != "trace-123" {
		t.Fatalf("GetTrace id=%q", trace.TraceID)
	}
	if !sawTrace {
		t.Fatal("expected fixture to receive /select/jaeger/api/traces/")
	}

	summaries, err := client.SearchTraces(ctx, tracing.TraceSearchRequest{Service: "frontend", Limit: 10})
	if err != nil {
		t.Fatalf("SearchTraces: %v", err)
	}
	if len(summaries) != 1 || summaries[0].TraceID != "trace-jaeger" {
		t.Fatalf("SearchTraces=%#v", summaries)
	}

	services, err := client.Services(ctx)
	if err != nil {
		t.Fatalf("Services: %v", err)
	}
	if !slices.Equal(services, []string{"frontend"}) {
		t.Fatalf("Services=%#v", services)
	}
	if !sawJaegerServices {
		t.Fatal("expected Services to hit Jaeger services endpoint")
	}

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if !sawHealth {
		t.Fatal("expected TestConnection to hit /health")
	}
}

func TestTestConnection_usesHealthThenReadyFallback(t *testing.T) {
	t.Parallel()

	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/ready" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)

	client, err := New(srv.URL, srv.Client())
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.TestConnection(ctx); err != nil {
		t.Fatalf("TestConnection: %v", err)
	}
	if len(paths) < 2 || paths[0] != "/health" {
		t.Fatalf("paths=%v, want /health then /ready", paths)
	}
}
