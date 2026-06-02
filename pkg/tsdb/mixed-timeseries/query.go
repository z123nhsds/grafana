package mixedtimeseries

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type prometheusResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]interface{} `json:"metric"`
			Value  []interface{}          `json:"value"`
		} `json:"result"`
	} `json:"data"`
}

type lokiResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]interface{} `json:"stream"`
			Values [][]string             `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

func (s *Service) queryPrometheus(ctx context.Context, dsInfo *datasourceInfo, expr string, query backend.DataQuery) (*data.Frame, error) {
	if expr == "" {
		return data.NewFrame("prometheus"), nil
	}

	baseURL := dsInfo.PrometheusURL
	if baseURL == "" {
		baseURL = "http://localhost:9090"
	}

	params := url.Values{}
	params.Set("query", expr)
	params.Set("start", fmt.Sprintf("%d", query.TimeRange.From.Unix()))
	params.Set("end", fmt.Sprintf("%d", query.TimeRange.To.Unix()))
	params.Set("step", fmt.Sprintf("%ds", int(query.MaxDataPoints)))

	reqURL := fmt.Sprintf("%s/api/v1/query_range?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := dsInfo.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var promResp prometheusResponse
	if err := json.Unmarshal(body, &promResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	frame := data.NewFrame("prometheus",
		data.NewField("time", nil, []time.Time{}),
		data.NewField("value", nil, []float64{}),
	)
	frame.Meta = &data.FrameMeta{
		Custom: map[string]interface{}{
			"source": "prometheus",
		},
	}

	for _, result := range promResp.Data.Result {
		for _, value := range result.Value {
			if valueArr, ok := value.([]interface{}); ok && len(valueArr) == 2 {
				if ts, ok := valueArr[0].(float64); ok {
					if val, ok := valueArr[1].(string); ok {
						var fval float64
						fmt.Sscanf(val, "%f", &fval)
						frame.AppendRow(time.Unix(int64(ts), 0), fval)
					}
				}
			}
		}
	}

	return frame, nil
}

func (s *Service) queryLoki(ctx context.Context, dsInfo *datasourceInfo, expr string, query backend.DataQuery) (*data.Frame, error) {
	if expr == "" {
		return data.NewFrame("loki"), nil
	}

	baseURL := dsInfo.LokiURL
	if baseURL == "" {
		baseURL = "http://localhost:3100"
	}

	params := url.Values{}
	params.Set("query", expr)
	params.Set("start", fmt.Sprintf("%d", query.TimeRange.From.UnixNano()))
	params.Set("end", fmt.Sprintf("%d", query.TimeRange.To.UnixNano()))
	params.Set("limit", "1000")

	reqURL := fmt.Sprintf("%s/loki/api/v1/query_range?%s", baseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := dsInfo.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var lokiResp lokiResponse
	if err := json.Unmarshal(body, &lokiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	frame := data.NewFrame("loki",
		data.NewField("time", nil, []time.Time{}),
		data.NewField("value", nil, []float64{}),
	)
	frame.Meta = &data.FrameMeta{
		Custom: map[string]interface{}{
			"source": "loki",
		},
	}

	for _, result := range lokiResp.Data.Result {
		for _, values := range result.Values {
			if len(values) == 2 {
				if ts, err := strconv.ParseInt(values[0], 10, 64); err == nil {
					if val, err := strconv.ParseFloat(values[1], 64); err == nil {
						frame.AppendRow(time.Unix(0, ts), val)
					}
				}
			}
		}
	}

	return frame, nil
}
