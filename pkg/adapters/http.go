package adapters

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/tidwall/gjson"
)

// HTTPAdapter is a generic HTTP adapter that can call any REST API endpoint
// and extract time-series data using JSON path expressions.
//
// It supports:
//   - Configurable HTTP method (GET, POST, etc.)
//   - Template-based request body with variables: {{.WindowSeconds}}, {{.Start}}, {{.End}}, {{.Step}}
//   - Custom headers including authentication (Bearer tokens, API keys, etc.)
//   - JSON path extraction for timestamps and values using gjson syntax
//   - Flexible timestamp parsing (RFC3339, Unix seconds, Unix milliseconds)
//
// Example configuration for a custom metrics API:
//
//	adapter := &HTTPAdapter{
//	    URL: "https://api.example.com/metrics",
//	    Method: "POST",
//	    Headers: map[string]string{
//	        "Authorization": "Bearer {{.Token}}",
//	        "Content-Type": "application/json",
//	    },
//	    Body: `{"metric": "requests", "window": "{{.WindowSeconds}}s"}`,
//	    ValuePath: "data.#.value",
//	    TimestampPath: "data.#.timestamp",
//	}
type HTTPAdapter struct {
	// URL is the endpoint to call (required)
	URL string

	// Method is the HTTP method (GET, POST, etc.). Defaults to GET if empty.
	Method string

	// Headers are custom HTTP headers to include in the request.
	// Values can use template variables like {{.Token}}.
	Headers map[string]string

	// Body is the request body template (for POST/PUT). Supports variables:
	//   {{.WindowSeconds}} - the collection window in seconds
	//   {{.Start}}         - start time as Unix timestamp
	//   {{.End}}           - end time as Unix timestamp
	//   {{.Step}}          - step size in seconds
	//   {{.StartRFC3339}}  - start time as RFC3339 string
	//   {{.EndRFC3339}}    - end time as RFC3339 string
	Body string

	// ValuePath is the gjson path to extract metric values from the response.
	// Use "#" for arrays, e.g. "data.#.value" extracts all values from data array.
	ValuePath string

	// TimestampPath is the gjson path to extract timestamps from the response.
	// Must return the same number of elements as ValuePath.
	TimestampPath string

	// TimestampFormat specifies how to parse timestamps:
	//   "rfc3339"    - RFC3339 strings (default)
	//   "unix"       - Unix seconds (float or int)
	//   "unix_milli" - Unix milliseconds (float or int)
	TimestampFormat string

	// StepSeconds controls the resolution (defaults to 60s if <= 0).
	StepSeconds int

	// HTTPClient is optional; if nil a default client with timeout is used.
	HTTPClient *http.Client

	// TemplateVars are custom variables available in Body and Headers templates.
	// Use this to pass tokens, API keys, etc.
	TemplateVars map[string]string
}

func (h *HTTPAdapter) Name() string { return "http" }

// Collect implements Adapter. It calls the configured HTTP endpoint and extracts
// time-series data using the configured JSON paths.
func (h *HTTPAdapter) Collect(ctx context.Context, windowSeconds int) (*DataFrame, error) {
	if h.URL == "" {
		return &DataFrame{}, errors.New("http adapter: URL is required")
	}
	if h.ValuePath == "" || h.TimestampPath == "" {
		return &DataFrame{}, errors.New("http adapter: ValuePath and TimestampPath are required")
	}

	step := h.StepSeconds
	if step <= 0 {
		step = 60
	}

	now := time.Now().UTC().Truncate(time.Second)
	start := now.Add(-time.Duration(windowSeconds) * time.Second)

	templateData := map[string]any{
		"WindowSeconds": windowSeconds,
		"Start":         start.Unix(),
		"End":           now.Unix(),
		"Step":          step,
		"StartRFC3339":  start.Format(time.RFC3339),
		"EndRFC3339":    now.Format(time.RFC3339),
	}

	for k, v := range h.TemplateVars {
		templateData[k] = v
	}

	method := h.Method
	if method == "" {
		method = http.MethodGet
	}

	var bodyReader io.Reader
	if h.Body != "" {
		renderedBody, err := renderTemplate(h.Body, templateData)
		if err != nil {
			return &DataFrame{}, fmt.Errorf("render body template: %w", err)
		}
		bodyReader = bytes.NewBufferString(renderedBody)
	}

	cli := h.HTTPClient
	if cli == nil {
		cli = &http.Client{Timeout: 10 * time.Second}
	}

	req, err := http.NewRequestWithContext(ctx, method, h.URL, bodyReader)
	if err != nil {
		return &DataFrame{}, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	for key, value := range h.Headers {
		rendered, err := renderTemplate(value, templateData)
		if err != nil {
			return &DataFrame{}, fmt.Errorf("render header %s: %w", key, err)
		}
		req.Header.Set(key, rendered)
	}

	resp, err := cli.Do(req)
	if err != nil {
		return &DataFrame{}, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return &DataFrame{}, fmt.Errorf("http status %d: %s", resp.StatusCode, string(body))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return &DataFrame{}, fmt.Errorf("read response: %w", err)
	}

	values := gjson.GetBytes(respBody, h.ValuePath)
	timestamps := gjson.GetBytes(respBody, h.TimestampPath)

	if !values.Exists() {
		return &DataFrame{}, fmt.Errorf("value path %q not found in response", h.ValuePath)
	}
	if !timestamps.Exists() {
		return &DataFrame{}, fmt.Errorf("timestamp path %q not found in response", h.TimestampPath)
	}

	valArray := values.Array()
	tsArray := timestamps.Array()

	if len(valArray) != len(tsArray) {
		return &DataFrame{}, fmt.Errorf("value count (%d) != timestamp count (%d)", len(valArray), len(tsArray))
	}

	rows := make([]Row, 0, len(valArray))
	for i := range valArray {
		val := valArray[i].Float()

		ts, err := h.parseTimestamp(tsArray[i])
		if err != nil {
			return &DataFrame{}, fmt.Errorf("parse timestamp[%d]: %w", i, err)
		}

		rows = append(rows, Row{
			"ts":    ts,
			"value": val,
		})
	}

	sort.Slice(rows, func(i, j int) bool {
		return rows[i]["ts"].(time.Time).Before(rows[j]["ts"].(time.Time))
	})

	for i := range rows {
		rows[i]["ts"] = rows[i]["ts"].(time.Time).UTC().Format(time.RFC3339)
	}

	return &DataFrame{Rows: rows}, nil
}

// parseTimestamp parses a timestamp according to the configured format
func (h *HTTPAdapter) parseTimestamp(value gjson.Result) (time.Time, error) {
	format := h.TimestampFormat
	if format == "" {
		format = "rfc3339"
	}

	switch format {
	case "rfc3339":
		return time.Parse(time.RFC3339, value.String())

	case "unix":
		// Unix seconds (supports both int and float)
		sec := value.Float()
		return time.Unix(int64(sec), 0).UTC(), nil

	case "unix_milli":
		ms := value.Float()
		return time.UnixMilli(int64(ms)).UTC(), nil

	default:
		return time.Time{}, fmt.Errorf("unsupported timestamp format: %s", format)
	}
}

// renderTemplate renders a text template with the given data
func renderTemplate(tmplStr string, data map[string]any) (string, error) {
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr, nil
	}

	tmpl, err := template.New("").Parse(tmplStr)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// ParseHTTPAdapterConfig creates an HTTPAdapter from a generic config map.
// This is useful for dynamic configuration from YAML/JSON.
//
// Example config:
//
//	{
//	  "url": "https://api.example.com/metrics",
//	  "method": "POST",
//	  "headers": {"Authorization": "Bearer token123"},
//	  "body": "{\"window\": \"{{.WindowSeconds}}s\"}",
//	  "valuePath": "data.#.value",
//	  "timestampPath": "data.#.ts",
//	  "timestampFormat": "rfc3339",
//	  "stepSeconds": 60
//	}
func ParseHTTPAdapterConfig(config map[string]any) (*HTTPAdapter, error) {
	adapter := &HTTPAdapter{
		TemplateVars: make(map[string]string),
	}

	if v, ok := config["url"].(string); ok {
		adapter.URL = v
	}
	if v, ok := config["method"].(string); ok {
		adapter.Method = v
	}
	if v, ok := config["body"].(string); ok {
		adapter.Body = v
	}
	if v, ok := config["valuePath"].(string); ok {
		adapter.ValuePath = v
	}
	if v, ok := config["timestampPath"].(string); ok {
		adapter.TimestampPath = v
	}
	if v, ok := config["timestampFormat"].(string); ok {
		adapter.TimestampFormat = v
	}

	if headers, ok := config["headers"].(map[string]any); ok {
		adapter.Headers = make(map[string]string)
		for k, v := range headers {
			if str, ok := v.(string); ok {
				adapter.Headers[k] = str
			}
		}
	}

	if v, ok := config["stepSeconds"]; ok {
		switch val := v.(type) {
		case int:
			adapter.StepSeconds = val
		case float64:
			adapter.StepSeconds = int(val)
		case string:
			step, err := strconv.Atoi(val)
			if err != nil {
				return nil, fmt.Errorf("invalid stepSeconds: %w", err)
			}
			adapter.StepSeconds = step
		}
	}

	if vars, ok := config["templateVars"].(map[string]any); ok {
		for k, v := range vars {
			if str, ok := v.(string); ok {
				adapter.TemplateVars[k] = str
			}
		}
	}

	return adapter, nil
}

// MustParseHTTPAdapterConfig is like ParseHTTPAdapterConfig but panics on error.
// Useful for static configurations where errors indicate programmer bugs.
func MustParseHTTPAdapterConfig(config map[string]any) *HTTPAdapter {
	adapter, err := ParseHTTPAdapterConfig(config)
	if err != nil {
		panic(fmt.Sprintf("parse http adapter config: %v", err))
	}
	return adapter
}

// ValidateConfig checks if the adapter configuration is valid
func (h *HTTPAdapter) ValidateConfig() error {
	if h.URL == "" {
		return errors.New("url is required")
	}
	if h.ValuePath == "" {
		return errors.New("valuePath is required")
	}
	if h.TimestampPath == "" {
		return errors.New("timestampPath is required")
	}

	validFormats := map[string]bool{
		"":           true,
		"rfc3339":    true,
		"unix":       true,
		"unix_milli": true,
	}
	if !validFormats[h.TimestampFormat] {
		return fmt.Errorf("invalid timestampFormat: %s (must be rfc3339, unix, or unix_milli)", h.TimestampFormat)
	}

	return nil
}
