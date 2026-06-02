package mixedtimeseries

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

const (
	PluginID = "grafana-mixed-timeseries-datasource"
)

type Service struct {
	im     instancemgmt.InstanceManager
	logger log.Logger
}

var (
	_ backend.QueryDataHandler    = (*Service)(nil)
	_ backend.CallResourceHandler = (*Service)(nil)
)

func ProvideService(httpClientProvider *httpclient.Provider) *Service {
	logger := backend.NewLoggerWith("logger", "tsdb.mixedtimeseries")
	return &Service{
		im:     datasource.NewInstanceManager(newInstanceSettings(httpClientProvider)),
		logger: logger,
	}
}

type datasourceInfo struct {
	PrometheusURL string
	LokiURL       string
	HTTPClient    *http.Client
}

func newInstanceSettings(httpClientProvider *httpclient.Provider) datasource.InstanceFactoryFunc {
	return func(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
		opts, err := settings.HTTPClientOptions(ctx)
		if err != nil {
			return nil, backend.DownstreamError(fmt.Errorf("error reading settings: %w", err))
		}
		opts.ForwardHTTPHeaders = true

		client, err := httpClientProvider.New(opts)
		if err != nil {
			return nil, backend.DownstreamError(fmt.Errorf("error creating http client: %w", err))
		}

		var promURL, lokiURL string
		if settings.JSONData != nil {
			var jsonData map[string]interface{}
			if err := json.Unmarshal(settings.JSONData, &jsonData); err == nil {
				if v, ok := jsonData["prometheusUrl"].(string); ok {
					promURL = v
				}
				if v, ok := jsonData["lokiUrl"].(string); ok {
					lokiURL = v
				}
			}
		}

		model := &datasourceInfo{
			PrometheusURL: promURL,
			LokiURL:       lokiURL,
			HTTPClient:    client,
		}
		return model, nil
	}
}

func (s *Service) getDSInfo(ctx context.Context, pluginCtx backend.PluginContext) (*datasourceInfo, error) {
	i, err := s.im.Get(ctx, pluginCtx)
	if err != nil {
		return nil, backend.DownstreamError(fmt.Errorf("failed to get data source info: %w", err))
	}

	instance, ok := i.(*datasourceInfo)
	if !ok {
		return nil, backend.DownstreamError(fmt.Errorf("failed to cast data source info"))
	}

	return instance, nil
}

type QueryJSONModel struct {
	RefID        string `json:"refId"`
	PrometheusExpr string `json:"prometheusExpr"`
	LokiExpr     string `json:"lokiExpr"`
	AlignmentMs  int64  `json:"alignmentMs"`
}

func (s *Service) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	dsInfo, err := s.getDSInfo(ctx, req.PluginContext)
	logger := s.logger.FromContext(ctx)
	if err != nil {
		logger.Error("Failed to get data source info", "error", err)
		return nil, err
	}

	result := backend.NewQueryDataResponse()

	var wg sync.WaitGroup
	var mu sync.Mutex

	for _, q := range req.Queries {
		wg.Add(1)
		go func(query backend.DataQuery) {
			defer wg.Done()

			var queryModel QueryJSONModel
			if err := json.Unmarshal(query.JSON, &queryModel); err != nil {
				mu.Lock()
				result.Responses[query.RefID] = backend.DataResponse{
					Error: backend.DownstreamError(fmt.Errorf("failed to parse query: %w", err)),
				}
				mu.Unlock()
				return
			}

			alignmentMs := queryModel.AlignmentMs
			if alignmentMs <= 0 {
				alignmentMs = 1000
			}

			mergedFrame, err := s.executeMergedQuery(ctx, dsInfo, queryModel, query, alignmentMs)
			if err != nil {
				mu.Lock()
				result.Responses[query.RefID] = backend.DataResponse{
					Error: backend.DownstreamError(fmt.Errorf("failed to execute merged query: %w", err)),
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			result.Responses[query.RefID] = backend.DataResponse{
				Frames: data.Frames{mergedFrame},
			}
			mu.Unlock()
		}(q)
	}

	wg.Wait()
	return result, nil
}

func (s *Service) executeMergedQuery(ctx context.Context, dsInfo *datasourceInfo, queryModel QueryJSONModel, query backend.DataQuery, alignmentMs int64) (*data.Frame, error) {
	promFrame, err := s.queryPrometheus(ctx, dsInfo, queryModel.PrometheusExpr, query)
	if err != nil {
		return nil, fmt.Errorf("prometheus query failed: %w", err)
	}

	lokiFrame, err := s.queryLoki(ctx, dsInfo, queryModel.LokiExpr, query)
	if err != nil {
		return nil, fmt.Errorf("loki query failed: %w", err)
	}

	mergedFrame := alignAndMergeFrames(promFrame, lokiFrame, alignmentMs)
	return mergedFrame, nil
}

func (s *Service) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	dsInfo, err := s.getDSInfo(ctx, req.PluginContext)
	logger := s.logger.FromContext(ctx)
	if err != nil {
		logger.Error("Failed to get data source info", "error", err)
		return err
	}

	switch req.Path {
	case "health":
		return s.handleHealthCheck(ctx, dsInfo, sender)
	case "ping":
		resp := map[string]interface{}{
			"status":    "ok",
			"timestamp": time.Now().UnixMilli(),
		}
		body, _ := json.Marshal(resp)
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusOK,
			Headers: map[string][]string{
				"content-type": {"application/json"},
			},
			Body: body,
		})
	default:
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusNotFound,
			Body:   []byte("not found"),
		})
	}
}

func (s *Service) handleHealthCheck(ctx context.Context, dsInfo *datasourceInfo, sender backend.CallResourceResponseSender) error {
	resp := map[string]interface{}{
		"status": "ok",
		"prometheus": map[string]interface{}{
			"url": dsInfo.PrometheusURL,
		},
		"loki": map[string]interface{}{
			"url": dsInfo.LokiURL,
		},
	}
	body, _ := json.Marshal(resp)
	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusOK,
		Headers: map[string][]string{
			"content-type": {"application/json"},
		},
		Body: body,
	})
}
