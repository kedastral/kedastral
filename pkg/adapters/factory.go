package adapters

import (
	"encoding/json"
	"fmt"
)

// New creates an adapter based on kind and generic configuration map.
// This is the central extension point for adding new adapter types.
//
// Supported kinds:
//   - "prometheus": Prometheus adapter
//   - "victoriametrics": VictoriaMetrics adapter
//   - "http": Generic HTTP adapter
//
// Returns error if kind is unknown or required fields are missing.
func New(kind string, config map[string]string, stepSeconds int) (Adapter, error) {
	switch kind {
	case "prometheus":
		return newPrometheus(config, stepSeconds)
	case "victoriametrics":
		return newVictoriaMetrics(config, stepSeconds)
	case "http":
		return newHTTP(config, stepSeconds)
	default:
		return nil, fmt.Errorf("unknown adapter kind: %s (must be prometheus, victoriametrics, or http)", kind)
	}
}

// newPrometheus creates a Prometheus adapter from generic config.
func newPrometheus(config map[string]string, stepSeconds int) (Adapter, error) {
	query := config["query"]
	if query == "" {
		return nil, fmt.Errorf("prometheus adapter requires 'query' config")
	}

	url := config["url"]
	if url == "" {
		url = "http://localhost:9090"
	}

	return &PrometheusAdapter{
		ServerURL:   url,
		Query:       query,
		StepSeconds: stepSeconds,
	}, nil
}

// newVictoriaMetrics creates a VictoriaMetrics adapter from generic config.
func newVictoriaMetrics(config map[string]string, stepSeconds int) (Adapter, error) {
	query := config["query"]
	if query == "" {
		return nil, fmt.Errorf("victoriametrics adapter requires 'query' config")
	}

	url := config["url"]
	if url == "" {
		url = "http://localhost:8428"
	}

	return &VictoriaMetricsAdapter{
		ServerURL:   url,
		Query:       query,
		StepSeconds: stepSeconds,
	}, nil
}

// newHTTP creates a generic HTTP adapter from generic config.
func newHTTP(config map[string]string, stepSeconds int) (Adapter, error) {
	url := config["url"]
	if url == "" {
		return nil, fmt.Errorf("http adapter requires 'url' config")
	}

	valuePath := config["valuePath"]
	timestampPath := config["timestampPath"]
	if valuePath == "" || timestampPath == "" {
		return nil, fmt.Errorf("http adapter requires 'valuePath' and 'timestampPath' config")
	}

	method := config["method"]
	if method == "" {
		method = "GET"
	}

	timestampFormat := config["timestampFormat"]
	if timestampFormat == "" {
		timestampFormat = "rfc3339"
	}

	var headers map[string]string
	if headersJSON := config["headers"]; headersJSON != "" {
		if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
			return nil, fmt.Errorf("invalid 'headers' JSON: %w", err)
		}
	}

	var templateVars map[string]string
	if varsJSON := config["templateVars"]; varsJSON != "" {
		if err := json.Unmarshal([]byte(varsJSON), &templateVars); err != nil {
			return nil, fmt.Errorf("invalid 'templateVars' JSON: %w", err)
		}
	}

	return &HTTPAdapter{
		URL:             url,
		Method:          method,
		Headers:         headers,
		Body:            config["body"],
		ValuePath:       valuePath,
		TimestampPath:   timestampPath,
		TimestampFormat: timestampFormat,
		StepSeconds:     stepSeconds,
		TemplateVars:    templateVars,
	}, nil
}
