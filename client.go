package victoriatraces

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aceobservability/ace-datasource-tempo/tracing"
	"github.com/aceobservability/ace/backend/pkg/datasource"
)

// Type is the RegisterDatasource key Ace uses for this module.
const Type = "victoriatraces"

// Client implements the Ace VictoriaTraces query and tracing datasource.
type Client struct {
	url        string
	httpClient *http.Client
}

// New constructs a VictoriaTraces datasource client.
// httpClient is required so Ace can inject DatasourceClient (dial/redirect policy + auth).
func New(tracesURL string, httpClient *http.Client) (*Client, error) {
	if strings.TrimSpace(tracesURL) == "" {
		return nil, fmt.Errorf("datasource url is required")
	}
	if httpClient == nil {
		return nil, fmt.Errorf("http client is required")
	}

	return &Client{
		url:        tracesURL,
		httpClient: httpClient,
	}, nil
}

// HTTPClient returns the injected HTTP client. Ace SSRF tests inspect policy wiring.
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

func (c *Client) Query(ctx context.Context, query string, start, end time.Time, step time.Duration, limit int) (*datasource.QueryResult, error) {
	_ = ctx
	_ = query
	_ = start
	_ = end
	_ = step
	_ = limit

	return nil, fmt.Errorf("victoriatraces datasource does not support /query; use tracing endpoints")
}

func (c *Client) GetTrace(ctx context.Context, traceID string) (*tracing.Trace, error) {
	trimmedTraceID := strings.TrimSpace(traceID)
	if trimmedTraceID == "" {
		return nil, fmt.Errorf("trace id is required")
	}

	endpoints := []string{
		"/select/jaeger/api/traces/" + url.PathEscape(trimmedTraceID),
		"/api/traces/" + url.PathEscape(trimmedTraceID),
	}

	var lastErr error
	for _, endpoint := range endpoints {
		payload, err := tracing.DoTracingRequest(ctx, c.httpClient, c.url, http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}

		trace, err := tracing.ParseTrace(payload)
		if err != nil {
			lastErr = err
			continue
		}

		return trace, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("failed to fetch trace")
}

func (c *Client) SearchTraces(ctx context.Context, req tracing.TraceSearchRequest) ([]tracing.TraceSummary, error) {
	// VictoriaTraces Jaeger API requires a service name. When none is
	// provided, fetch the known services and query each one individually.
	if strings.TrimSpace(req.Service) == "" {
		services, err := c.Services(ctx)
		if err != nil || len(services) == 0 {
			return []tracing.TraceSummary{}, nil
		}

		var allTraces []tracing.TraceSummary
		for _, svc := range services {
			svcReq := req
			svcReq.Service = svc
			traces, err := c.searchTracesForService(ctx, svcReq)
			if err != nil {
				continue
			}
			allTraces = append(allTraces, traces...)
		}

		return tracing.NormalizeTraceSearchResults(allTraces, req.Limit), nil
	}

	return c.searchTracesForService(ctx, req)
}

func (c *Client) searchTracesForService(ctx context.Context, req tracing.TraceSearchRequest) ([]tracing.TraceSummary, error) {
	// VictoriaTraces Jaeger API expects timestamps in microseconds, not seconds.
	jaegerReq := req
	if jaegerReq.Start > 0 && jaegerReq.Start < 1e12 {
		jaegerReq.Start *= 1_000_000
	}
	if jaegerReq.End > 0 && jaegerReq.End < 1e12 {
		jaegerReq.End *= 1_000_000
	}
	params := tracing.BuildTraceSearchParams(jaegerReq)
	endpoint := "/select/jaeger/api/traces?" + params.Encode()

	payload, err := tracing.DoTracingRequest(ctx, c.httpClient, c.url, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	traces, err := tracing.ParseTraceSearchResponse(payload)
	if err != nil {
		return nil, err
	}

	return traces, nil
}

func (c *Client) Services(ctx context.Context) ([]string, error) {
	endpoints := []string{
		"/select/jaeger/api/services",
		"/api/services",
	}

	var lastErr error
	for _, endpoint := range endpoints {
		payload, err := tracing.DoTracingRequest(ctx, c.httpClient, c.url, http.MethodGet, endpoint, nil)
		if err != nil {
			lastErr = err
			continue
		}

		services, err := tracing.ParseStringSlicePayload(payload)
		if err != nil {
			lastErr = err
			continue
		}

		return services, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("failed to fetch trace services")
}

var (
	_ datasource.Client     = (*Client)(nil)
	_ tracing.TracingClient = (*Client)(nil)
)
