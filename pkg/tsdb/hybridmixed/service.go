package hybridmixed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	defaultStep       = int64(1000)
	defaultLokiLimit  = 1000
	defaultBatchSize  = 256
	defaultPushEvery  = int64(25)
	streamContentType = "application/x-ndjson"
)

type Service struct {
	client *http.Client
}

type datasourceSettings struct {
	PrometheusURL string `json:"prometheusUrl"`
	LokiURL       string `json:"lokiUrl"`
}

type queryModel struct {
	PrometheusExpr string `json:"prometheusExpr"`
	LokiExpr       string `json:"lokiExpr"`
	StepMs         int64  `json:"stepMs"`
	LokiLimit      int64  `json:"lokiLimit"`
}

type mergedPoint struct {
	Timestamp  int64    `json:"timestamp"`
	Prometheus *float64 `json:"prometheus"`
	Loki       *float64 `json:"loki"`
	Merged     *float64 `json:"merged"`
}

type promEnvelope struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string       `json:"resultType"`
		Result     []promResult `json:"result"`
	} `json:"data"`
}

type promResult struct {
	Metric map[string]string `json:"metric"`
	Values [][2]json.RawMessage `json:"values"`
	Value  [2]json.RawMessage `json:"value"`
}

type lokiEnvelope struct {
	Status string `json:"status"`
	Error  string `json:"error"`
	Data   struct {
		ResultType string            `json:"resultType"`
		Result     []json.RawMessage `json:"result"`
	} `json:"data"`
}

type lokiStreamsResult struct {
	Stream map[string]string `json:"stream"`
	Values [][2]string       `json:"values"`
}

type lokiMatrixResult struct {
	Metric map[string]string `json:"metric"`
	Values [][2]json.RawMessage `json:"values"`
	Value  [2]json.RawMessage `json:"value"`
}

func ProvideService() *Service {
	return &Service{
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (s *Service) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, query := range req.Queries {
		points, err := s.runMergedQuery(ctx, req.PluginContext, query)
		if err != nil {
			response.Responses[query.RefID] = backend.ErrorResponseWithErrorSource(err)
			continue
		}

		response.Responses[query.RefID] = backend.DataResponse{
			Frames: []*data.Frame{buildFrame(query.RefID, points)},
		}
	}

	return response, nil
}

func (s *Service) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	settings, err := readDatasourceSettings(req.PluginContext)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{Status: http.StatusBadRequest, Body: []byte(err.Error())})
	}

	switch req.Path {
	case "capabilities":
		payload, marshalErr := json.Marshal(map[string]any{
			"prometheusUrl": settings.PrometheusURL,
			"lokiUrl":       settings.LokiURL,
			"stream":        true,
			"merge":         "time-aligned",
		})
		if marshalErr != nil {
			return sender.Send(&backend.CallResourceResponse{Status: http.StatusInternalServerError, Body: []byte(marshalErr.Error())})
		}
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusOK,
			Headers: map[string][]string{
				"Content-Type": {"application/json"},
			},
			Body: payload,
		})
	case "stream":
		return s.streamMergedQuery(ctx, req, sender, settings)
	default:
		return sender.Send(&backend.CallResourceResponse{Status: http.StatusNotFound, Body: []byte("resource not found")})
	}
}

func (s *Service) streamMergedQuery(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender, settings datasourceSettings) error {
	model, queryRange, batchSize, pushEveryMs, err := readStreamRequest(req)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{Status: http.StatusBadRequest, Body: []byte(err.Error())})
	}

	points, err := s.executeMergedQuery(ctx, settings, model, queryRange)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{Status: http.StatusBadGateway, Body: []byte(err.Error())})
	}

	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	if pushEveryMs < 0 {
		pushEveryMs = 0
	}

	firstBatch := min(batchSize, len(points))
	initialBody := marshalPoints(points[:firstBatch])
	if err := sender.Send(&backend.CallResourceResponse{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type": {streamContentType},
		},
		Body: initialBody,
	}); err != nil {
		return err
	}

	for start := firstBatch; start < len(points); start += batchSize {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		end := min(start+batchSize, len(points))
		if err := sender.SendBytes(marshalPoints(points[start:end])); err != nil {
			return err
		}
		if pushEveryMs > 0 {
			time.Sleep(time.Duration(pushEveryMs) * time.Millisecond)
		}
	}

	return nil
}

func (s *Service) runMergedQuery(ctx context.Context, pluginContext backend.PluginContext, query backend.DataQuery) ([]mergedPoint, error) {
	settings, err := readDatasourceSettings(pluginContext)
	if err != nil {
		return nil, backend.PluginError(err)
	}

	var model queryModel
	if err := json.Unmarshal(query.JSON, &model); err != nil {
		return nil, backend.PluginError(fmt.Errorf("invalid query model: %w", err))
	}

	queryRange := backend.TimeRange{From: query.TimeRange.From, To: query.TimeRange.To}
	if queryRange.From.IsZero() || queryRange.To.IsZero() {
		queryRange = backend.TimeRange{From: time.Now().Add(-time.Hour), To: time.Now()}
	}

	points, err := s.executeMergedQuery(ctx, settings, model, queryRange)
	if err != nil {
		return nil, backend.DownstreamError(err)
	}

	return points, nil
}

func (s *Service) executeMergedQuery(ctx context.Context, settings datasourceSettings, model queryModel, queryRange backend.TimeRange) ([]mergedPoint, error) {
	stepMs := normalizeStep(model.StepMs, queryRange)
	if model.PrometheusExpr == "" && model.LokiExpr == "" {
		return nil, errors.New("at least one of prometheusExpr or lokiExpr is required")
	}

	promValues := map[int64]float64{}
	lokiValues := map[int64]float64{}

	if model.PrometheusExpr != "" {
		values, err := s.queryPrometheus(ctx, settings.PrometheusURL, model.PrometheusExpr, queryRange, stepMs)
		if err != nil {
			return nil, err
		}
		promValues = values
	}

	if model.LokiExpr != "" {
		values, err := s.queryLoki(ctx, settings.LokiURL, model.LokiExpr, queryRange, stepMs, model.LokiLimit)
		if err != nil {
			return nil, err
		}
		lokiValues = values
	}

	return mergeSeries(queryRange, stepMs, promValues, lokiValues), nil
}

func (s *Service) queryPrometheus(ctx context.Context, baseURL, expr string, queryRange backend.TimeRange, stepMs int64) (map[int64]float64, error) {
	if baseURL == "" {
		return nil, errors.New("prometheusUrl is required")
	}

	requestURL, err := buildURL(baseURL, "/api/v1/query_range", map[string]string{
		"query": expr,
		"start": formatPrometheusTime(queryRange.From),
		"end":   formatPrometheusTime(queryRange.To),
		"step":  formatStep(stepMs),
	})
	if err != nil {
		return nil, err
	}

	body, err := s.doRequest(ctx, requestURL)
	if err != nil {
		return nil, err
	}

	var envelope promEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to decode prometheus response: %w", err)
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("prometheus query failed: %s", envelope.Error)
	}

	values := make(map[int64]float64)
	for _, result := range envelope.Data.Result {
		if len(result.Values) > 0 {
			for _, sample := range result.Values {
				timestamp, value, err := parseTimestampValuePair(sample[0], sample[1], 1000)
				if err != nil {
					return nil, err
				}
				values[timestamp] += value
			}
			continue
		}

		if len(result.Value) == 2 {
			timestamp, value, err := parseTimestampValuePair(result.Value[0], result.Value[1], 1000)
			if err != nil {
				return nil, err
			}
			values[timestamp] += value
		}
	}

	return values, nil
}

func (s *Service) queryLoki(ctx context.Context, baseURL, expr string, queryRange backend.TimeRange, stepMs, limit int64) (map[int64]float64, error) {
	if baseURL == "" {
		return nil, errors.New("lokiUrl is required")
	}
	if limit <= 0 {
		limit = defaultLokiLimit
	}

	requestURL, err := buildURL(baseURL, "/loki/api/v1/query_range", map[string]string{
		"query":     expr,
		"start":     strconv.FormatInt(queryRange.From.UnixNano(), 10),
		"end":       strconv.FormatInt(queryRange.To.UnixNano(), 10),
		"limit":     strconv.FormatInt(limit, 10),
		"direction": "forward",
		"step":      formatStep(stepMs),
	})
	if err != nil {
		return nil, err
	}

	body, err := s.doRequest(ctx, requestURL)
	if err != nil {
		return nil, err
	}

	var envelope lokiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to decode loki response: %w", err)
	}
	if envelope.Status != "success" {
		return nil, fmt.Errorf("loki query failed: %s", envelope.Error)
	}

	values := make(map[int64]float64)
	switch envelope.Data.ResultType {
	case "streams":
		for _, rawResult := range envelope.Data.Result {
			var stream lokiStreamsResult
			if err := json.Unmarshal(rawResult, &stream); err != nil {
				return nil, fmt.Errorf("failed to decode loki streams result: %w", err)
			}
			for _, entry := range stream.Values {
				timestamp, err := strconv.ParseInt(entry[0], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("failed to parse loki stream timestamp: %w", err)
				}
				values[timestamp/1_000_000]++
			}
		}
	case "matrix", "vector":
		for _, rawResult := range envelope.Data.Result {
			var result lokiMatrixResult
			if err := json.Unmarshal(rawResult, &result); err != nil {
				return nil, fmt.Errorf("failed to decode loki metric result: %w", err)
			}
			if len(result.Values) > 0 {
				for _, sample := range result.Values {
					timestamp, value, err := parseTimestampValuePair(sample[0], sample[1], 1000)
					if err != nil {
						return nil, err
					}
					values[timestamp] += value
				}
				continue
			}
			if len(result.Value) == 2 {
				timestamp, value, err := parseTimestampValuePair(result.Value[0], result.Value[1], 1000)
				if err != nil {
					return nil, err
				}
				values[timestamp] += value
			}
		}
	default:
		return nil, fmt.Errorf("unsupported loki result type: %s", envelope.Data.ResultType)
	}

	return values, nil
}

func (s *Service) doRequest(ctx context.Context, requestURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("request failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return body, nil
}

func readDatasourceSettings(pluginContext backend.PluginContext) (datasourceSettings, error) {
	if pluginContext.DataSourceInstanceSettings == nil {
		return datasourceSettings{}, errors.New("missing datasource instance settings")
	}

	settings := datasourceSettings{}
	if len(pluginContext.DataSourceInstanceSettings.JSONData) > 0 {
		if err := json.Unmarshal(pluginContext.DataSourceInstanceSettings.JSONData, &settings); err != nil {
			return datasourceSettings{}, fmt.Errorf("invalid datasource jsonData: %w", err)
		}
	}

	if settings.PrometheusURL == "" {
		settings.PrometheusURL = pluginContext.DataSourceInstanceSettings.URL
	}

	return settings, nil
}

func readStreamRequest(req *backend.CallResourceRequest) (queryModel, backend.TimeRange, int, int64, error) {
	model := queryModel{
		PrometheusExpr: req.URL.Query().Get("prometheusExpr"),
		LokiExpr:       req.URL.Query().Get("lokiExpr"),
		StepMs:         parseInt64(req.URL.Query().Get("stepMs"), 0),
		LokiLimit:      parseInt64(req.URL.Query().Get("lokiLimit"), defaultLokiLimit),
	}

	from := parseInt64(req.URL.Query().Get("from"), time.Now().Add(-time.Hour).UnixMilli())
	to := parseInt64(req.URL.Query().Get("to"), time.Now().UnixMilli())
	batchSize := int(parseInt64(req.URL.Query().Get("batchSize"), defaultBatchSize))
	pushEveryMs := parseInt64(req.URL.Query().Get("pushEveryMs"), defaultPushEvery)

	if from > to {
		return queryModel{}, backend.TimeRange{}, 0, 0, errors.New("from must be less than or equal to to")
	}

	return model, backend.TimeRange{From: time.UnixMilli(from), To: time.UnixMilli(to)}, batchSize, pushEveryMs, nil
}

func mergeSeries(queryRange backend.TimeRange, stepMs int64, promValues, lokiValues map[int64]float64) []mergedPoint {
	startMs := queryRange.From.UnixMilli()
	endMs := queryRange.To.UnixMilli()
	if endMs < startMs {
		startMs, endMs = endMs, startMs
	}

	promBuckets := bucketValues(startMs, stepMs, promValues)
	lokiBuckets := bucketValues(startMs, stepMs, lokiValues)
	points := make([]mergedPoint, 0, int(((endMs-startMs)/stepMs)+1))

	for timestamp := startMs; timestamp <= endMs; timestamp += stepMs {
		point := mergedPoint{Timestamp: timestamp}
		promValue, hasProm := promBuckets[timestamp]
		if hasProm {
			point.Prometheus = floatPointer(promValue)
		}
		lokiValue, hasLoki := lokiBuckets[timestamp]
		if hasLoki {
			point.Loki = floatPointer(lokiValue)
		}
		if hasProm || hasLoki {
			point.Merged = floatPointer(promValue + lokiValue)
		}
		points = append(points, point)
	}

	return points
}

func bucketValues(startMs, stepMs int64, values map[int64]float64) map[int64]float64 {
	if len(values) == 0 {
		return map[int64]float64{}
	}

	bucketed := make(map[int64]float64, len(values))
	keys := make([]int64, 0, len(values))
	for timestamp := range values {
		keys = append(keys, timestamp)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })

	for _, timestamp := range keys {
		bucket := startMs
		if timestamp > startMs {
			bucket = startMs + ((timestamp-startMs)/stepMs)*stepMs
		}
		bucketed[bucket] += values[timestamp]
	}

	return bucketed
}

func buildFrame(refID string, points []mergedPoint) *data.Frame {
	times := make([]time.Time, 0, len(points))
	prometheusValues := make([]*float64, 0, len(points))
	lokiValues := make([]*float64, 0, len(points))
	mergedValues := make([]*float64, 0, len(points))

	for _, point := range points {
		times = append(times, time.UnixMilli(point.Timestamp))
		prometheusValues = append(prometheusValues, point.Prometheus)
		lokiValues = append(lokiValues, point.Loki)
		mergedValues = append(mergedValues, point.Merged)
	}

	frame := data.NewFrame(refID,
		data.NewField("time", nil, times),
		data.NewField("prometheus", nil, prometheusValues),
		data.NewField("loki", nil, lokiValues),
		data.NewField("merged", nil, mergedValues),
	)
	frame.RefID = refID
	frame.Meta = &data.FrameMeta{PreferredVisualization: "table"}
	return frame
}

func marshalPoints(points []mergedPoint) []byte {
	if len(points) == 0 {
		return nil
	}

	var builder strings.Builder
	for _, point := range points {
		payload, _ := json.Marshal(point)
		builder.Write(payload)
		builder.WriteByte('\n')
	}
	return []byte(builder.String())
}

func parseTimestampValuePair(rawTimestamp, rawValue json.RawMessage, multiplier int64) (int64, float64, error) {
	timestampValue, err := parseFloat(rawTimestamp)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	value, err := parseFloat(rawValue)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse value: %w", err)
	}
	return int64(math.Round(timestampValue * float64(multiplier))), value, nil
}

func parseFloat(raw json.RawMessage) (float64, error) {
	var numeric float64
	if err := json.Unmarshal(raw, &numeric); err == nil {
		return numeric, nil
	}

	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, err
	}
	return strconv.ParseFloat(value, 64)
}

func buildURL(baseURL, path string, queryParams map[string]string) (string, error) {
	parsedURL, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return "", err
	}
	parsedURL.Path = strings.TrimRight(parsedURL.Path, "/") + path
	values := parsedURL.Query()
	for key, value := range queryParams {
		if value != "" {
			values.Set(key, value)
		}
	}
	parsedURL.RawQuery = values.Encode()
	return parsedURL.String(), nil
}

func normalizeStep(stepMs int64, queryRange backend.TimeRange) int64 {
	if stepMs > 0 {
		return stepMs
	}
	rangeMs := queryRange.To.UnixMilli() - queryRange.From.UnixMilli()
	if rangeMs <= 0 {
		return defaultStep
	}
	calculated := rangeMs / 1000
	if calculated <= 0 {
		return defaultStep
	}
	return calculated
}

func formatPrometheusTime(value time.Time) string {
	seconds := float64(value.UnixNano()) / float64(time.Second)
	return strconv.FormatFloat(seconds, 'f', -1, 64)
}

func formatStep(stepMs int64) string {
	seconds := float64(stepMs) / 1000
	return strconv.FormatFloat(seconds, 'f', -1, 64)
}

func parseInt64(value string, fallback int64) int64 {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func floatPointer(value float64) *float64 {
	v := value
	return &v
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
