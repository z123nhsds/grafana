package multimix

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	pluginID = "multimix"
)

type Service struct {
	httpClientProvider *httpclient.Provider
	logger             log.Logger
	streams            map[string]*streamState
	streamsMu          sync.RWMutex
}

type streamState struct {
	frames  data.FrameJSONCache
	cancel  context.CancelFunc
}

var (
	_ backend.QueryDataHandler    = (*Service)(nil)
	_ backend.CallResourceHandler = (*Service)(nil)
)

func ProvideService(httpClientProvider *httpclient.Provider) *Service {
	logger := backend.NewLoggerWith("logger", "tsdb.multimix")
	logger.Debug("Initializing multimix datasource")
	return &Service{
		httpClientProvider: httpClientProvider,
		logger:             logger,
		streams:            make(map[string]*streamState),
	}
}

type multimixQueryModel struct {
	PrometheusQuery string `json:"prometheusQuery"`
	LokiQuery       string `json:"lokiQuery"`
	Step            string `json:"step"`
	MergeMode       string `json:"mergeMode"`
}

type mergedTimePoint struct {
	Timestamp    time.Time
	PromValue    *float64
	LokiLogCount *float64
}

func (s *Service) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	result := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		var model multimixQueryModel
		if err := json.Unmarshal(q.JSON, &model); err != nil {
			result.Responses[q.RefID] = backend.DataResponse{
				Error: fmt.Errorf("failed to parse query model: %w", err),
			}
			continue
		}

		resp, err := s.handleQuery(ctx, req, q.RefID, model)
		if err != nil {
			result.Responses[q.RefID] = backend.DataResponse{
				Error: err,
			}
			continue
		}
		result.Responses[q.RefID] = *resp
	}

	return result, nil
}

func (s *Service) handleQuery(ctx context.Context, req *backend.QueryDataRequest, refID string, model multimixQueryModel) (*backend.DataResponse, error) {
	from := req.Queries[0].TimeRange.From
	to := req.Queries[0].TimeRange.To

	step := 15 * time.Second
	if model.Step != "" {
		if parsed, err := time.ParseDuration(model.Step); err == nil {
			step = parsed
		}
	}

	if interval := req.Queries[0].Interval; interval > 0 {
		step = time.Duration(interval) * time.Second
	}

	var promFrames data.Frames
	var lokiFrames data.Frames
	var promErr, lokiErr error
	var wg sync.WaitGroup
	var mu sync.Mutex

	if model.PrometheusQuery != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frames, err := s.queryPrometheus(ctx, req, model.PrometheusQuery, from, to, step)
			mu.Lock()
			promFrames = frames
			promErr = err
			mu.Unlock()
		}()
	}

	if model.LokiQuery != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			frames, err := s.queryLoki(ctx, req, model.LokiQuery, from, to, step)
			mu.Lock()
			lokiFrames = frames
			lokiErr = err
			mu.Unlock()
		}()
	}

	wg.Wait()

	if promErr != nil && lokiErr != nil {
		return nil, fmt.Errorf("both prometheus and loki queries failed: prometheus: %w, loki: %w", promErr, lokiErr)
	}

	merged := mergeTimeSeries(promFrames, lokiFrames, from, to, step)
	frame := buildDataFrame(merged, refID)

	return &backend.DataResponse{
		Frames: data.Frames{frame},
	}, nil
}

func (s *Service) queryPrometheus(ctx context.Context, req *backend.QueryDataRequest, query string, from, to time.Time, step time.Duration) (data.Frames, error) {
	promReq := &backend.QueryDataRequest{
		PluginContext: req.PluginContext,
		Headers:       req.Headers,
		Queries: []backend.DataQuery{
			{
				RefID:     "prom",
				QueryType: "timeSeriesQuery",
				Interval:  step.Milliseconds() / 1000,
				TimeRange: backend.TimeRange{From: from, To: to},
				JSON:      mustMarshal(map[string]interface{}{"expr": query, "legendFormat": "__auto"}),
			},
		},
	}

	dsSettings := backend.DataSourceInstanceSettings{
		UID:                      req.PluginContext.DataSourceInstanceSettings.UID + "-prom",
		Name:                     "prometheus",
		Type:                     "prometheus",
		URL:                      req.PluginContext.DataSourceInstanceSettings.URL,
		JSONData:                 req.PluginContext.DataSourceInstanceSettings.JSONData,
		DecryptedSecureJSONData:  req.PluginContext.DataSourceInstanceSettings.DecryptedSecureJSONData,
	}
	promReq.PluginContext.DataSourceInstanceSettings = &dsSettings

	return nil, nil
}

func (s *Service) queryLoki(ctx context.Context, req *backend.QueryDataRequest, query string, from, to time.Time, step time.Duration) (data.Frames, error) {
	return nil, nil
}

func (s *Service) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	switch req.Path {
	case "health":
		return s.callHealth(ctx, req, sender)
	case "query":
		return s.callResourceQuery(ctx, req, sender)
	case "stream":
		return s.callStream(ctx, req, sender)
	default:
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
			Body:   []byte(`{"error": "unknown resource path"}`),
		})
	}
}

func (s *Service) callHealth(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body: []byte(`{"status":"ok"}`),
	})
}

func (s *Service) callResourceQuery(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if req.Method != http.MethodPost {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusMethodNotAllowed,
			Body:   []byte(`{"error": "method not allowed"}`),
		})
	}

	var model multimixQueryModel
	if err := json.Unmarshal(req.Body, &model); err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusBadRequest,
			Body:   []byte(fmt.Sprintf(`{"error": "invalid request body: %s"}`, err.Error())),
		})
	}

	now := time.Now()
	from := now.Add(-1 * time.Hour)
	to := now

	step := 15 * time.Second
	if model.Step != "" {
		if parsed, err := time.ParseDuration(model.Step); err == nil {
			step = parsed
		}
	}

	points := generateMergedPoints(from, to, step)

	frame := buildDataFrameFromPoints(points, "resource")
	resp := backend.DataResponse{Frames: data.Frames{frame}}

	respBytes, err := json.Marshal(resp)
	if err != nil {
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusInternalServerError,
			Body:   []byte(fmt.Sprintf(`{"error": "failed to marshal response: %s"}`, err.Error())),
		})
	}

	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"Content-Type": {"application/json"},
		},
		Body: respBytes,
	})
}

func (s *Service) callStream(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	streamID := req.URL

	s.streamsMu.Lock()
	if _, exists := s.streams[streamID]; exists {
		s.streamsMu.Unlock()
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusConflict,
			Body:   []byte(`{"error": "stream already exists"}`),
		})
	}

	streamCtx, cancel := context.WithCancel(ctx)
	s.streams[streamID] = &streamState{
		frames: data.FrameJSONCache{},
		cancel: cancel,
	}
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()
		delete(s.streams, streamID)
		s.streamsMu.Unlock()
		cancel()
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	startTime := time.Now()

	for {
		select {
		case <-streamCtx.Done():
			return nil
		case t := <-ticker.C:
			elapsed := t.Sub(startTime)
			points := generateMergedPoints(startTime, startTime.Add(elapsed), 5*time.Second)
			frame := buildDataFrameFromPoints(points, "stream")

			frameBytes, err := json.Marshal(frame)
			if err != nil {
				s.logger.Error("Failed to marshal stream frame", "error", err)
				continue
			}

			err = sender.Send(&backend.CallResourceResponse{
				Status: http.StatusOK,
				Headers: map[string][]string{
					"Content-Type": {"application/json"},
				},
				Body: frameBytes,
			})
			if err != nil {
				s.logger.Error("Failed to send stream data", "error", err)
				return nil
			}
		}
	}
}

func generateMergedPoints(from, to time.Time, step time.Duration) []mergedTimePoint {
	var points []mergedTimePoint
	for t := from; t.Before(to) || t.Equal(to); t = t.Add(step) {
		ts := t
		promVal := float64(ts.Unix()%100) * 0.5
		lokiVal := float64(ts.Unix()%50) * 0.3
		points = append(points, mergedTimePoint{
			Timestamp:    ts,
			PromValue:    &promVal,
			LokiLogCount: &lokiVal,
		})
	}
	return points
}

func buildDataFrameFromPoints(points []mergedTimePoint, name string) *data.Frame {
	timestamps := make([]time.Time, len(points))
	promValues := make([]*float64, len(points))
	lokiValues := make([]*float64, len(points))

	for i, p := range points {
		timestamps[i] = p.Timestamp
		promValues[i] = p.PromValue
		lokiValues[i] = p.LokiLogCount
	}

	frame := data.NewFrame(name,
		data.NewField("time", nil, timestamps),
		data.NewField("prometheus_value", nil, promValues),
		data.NewField("loki_log_count", nil, lokiValues),
	)

	return frame
}

func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}