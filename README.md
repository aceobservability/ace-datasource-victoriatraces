# ace-datasource-victoriatraces

Compile-time VictoriaTraces datasource module for [Ace](https://github.com/aceobservability/ace).

Ace keeps the datasource contract and registry in
`github.com/aceobservability/ace/backend/pkg/datasource`. This module implements
that `Client` plus VictoriaTraces tracing (`GetTrace`, `SearchTraces`,
`Services`). Ace registers the factory at `init` and injects its SSRF-safe HTTP
client — this module does not import Ace `internal/` packages and does not
construct an unpolicy'd client.

Shared Tempo/Jaeger parse helpers come from
`github.com/aceobservability/ace-datasource-tempo/tracing`.

## Contract

| Surface | Package |
| --- | --- |
| Query / result types | `github.com/aceobservability/ace/backend/pkg/datasource` |
| Trace types / helpers | `github.com/aceobservability/ace-datasource-tempo/tracing` |
| Registry type key | `victoriatraces` (`Type`) |
| Factory | `New(url string, httpClient *http.Client)` |

`httpClient` is required. Ace passes `ssrf.DatasourceClient` wrapped with stored
datasource credentials.

## Tests

```
go test ./...
```

Query and connection tests speak to an `httptest` fixture. No live VictoriaTraces
is required.
