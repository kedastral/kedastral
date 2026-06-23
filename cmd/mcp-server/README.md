# Kedastral MCP Server

The MCP server exposes Kedastral forecast data as [Model Context Protocol](https://modelcontextprotocol.io/) tools, enabling AI assistants to query predictions and understand scaling decisions in natural language.

## Tools

| Tool | Description |
|------|-------------|
| `list_workloads` | List all workloads currently tracked by the forecaster |
| `get_forecast` | Get the full forecast snapshot for a workload (metric values, desired replicas, freshness) |
| `explain_decision` | Human-readable explanation of the current scaling decision (trend, peak, staleness warning) |

### Operator-aware tools

When the server has Kubernetes access (in-cluster, or a kubeconfig for local use), it
also registers tools that read [operator](../../docs/OPERATOR.md) resources. These are
skipped automatically when no cluster is reachable.

| Tool | Description |
|------|-------------|
| `list_forecast_policies` | List `ForecastPolicy` resources with target, model, readiness, and current/desired replicas |
| `get_forecast_policy` | Full spec and status for one `ForecastPolicy`, including readiness conditions and the generated `ScaledObject` |

In-cluster, the Helm chart grants the MCP server read-only RBAC on
`forecastpolicies`/`datasources` when `mcpServer.enabled` is set.

## Transport modes

The server supports two transports selectable via `--transport`:

| Mode | Use case | How it works |
|------|----------|--------------|
| `stdio` (default) | Local AI clients (Claude Desktop, Cursor) | Spawned as a subprocess; communicates over stdin/stdout |
| `sse` | Cluster deployments | Long-running HTTP service; AI clients connect to the SSE endpoint |

## Local usage (stdio)

Build the binary and register it with your AI client.

```bash
make mcp-server
```

**Claude Desktop** (`~/.claude/claude_desktop_config.json`):
```json
{
  "mcpServers": {
    "kedastral": {
      "command": "/path/to/bin/mcp-server",
      "args": ["-forecaster-url=http://localhost:8081"]
    }
  }
}
```

Port-forward the forecaster if it is running in-cluster:
```bash
kubectl port-forward svc/kedastral-forecaster 8081:8081
```

## Cluster deployment (SSE)

Enable the MCP server in the Helm chart:

```yaml
mcpServer:
  enabled: true
  config:
    baseURL: "http://kedastral-mcp-server.your-domain.com"
    forecasterURL: "http://kedastral-forecaster:8081"
```

Then point your AI client at the SSE endpoint:

```json
{
  "mcpServers": {
    "kedastral": {
      "url": "http://kedastral-mcp-server.your-domain.com/sse"
    }
  }
}
```

## Configuration

| Flag | Env var | Default | Description |
|------|---------|---------|-------------|
| `--forecaster-url` | `FORECASTER_URL` | `http://localhost:8081` | Forecaster HTTP endpoint |
| `--transport` | `MCP_TRANSPORT` | `stdio` | Transport mode: `stdio` or `sse` |
| `--listen` | `MCP_LISTEN` | `:8083` | Listen address for SSE transport |
| `--base-url` | `MCP_BASE_URL` | _(required for sse)_ | Public base URL for SSE event streams |
| `--stale-after` | `STALE_AFTER` | `5m` | Age threshold beyond which a snapshot is flagged as stale |
| `--log-level` | `LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `--log-format` | `LOG_FORMAT` | `text` | Log format: `text` or `json` |
