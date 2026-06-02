package mixeddatasource

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type Service struct {
	im instancemgmt.InstanceManager
}

func ProvideService() *Service {
	return &Service{
		im: instancemgmt.New(newInstanceSettings),
	}
}

func newInstanceSettings(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	return &instance{}, nil
}

type instance struct{}

func (s *Service) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		res := s.query(ctx, req.PluginContext, q)
		response.Responses[q.RefID] = res
	}

	return response, nil
}

func (s *Service) query(_ context.Context, pCtx backend.PluginContext, query backend.DataQuery) backend.DataResponse {
	response := backend.DataResponse{}

	// Simulate merging Prometheus and Loki results in memory with time alignment
	frame := data.NewFrame("merged_data")

	times := []time.Time{
		time.Now().Add(-5 * time.Minute),
		time.Now().Add(-4 * time.Minute),
		time.Now().Add(-3 * time.Minute),
		time.Now().Add(-2 * time.Minute),
		time.Now().Add(-1 * time.Minute),
	}

	promValues := []float64{1.2, 1.4, 2.1, 1.9, 2.5}
	lokiLogs := []string{"info", "error", "info", "info", "warn"}

	frame.Fields = append(frame.Fields,
		data.NewField("time", nil, times),
		data.NewField("prometheus_metric", nil, promValues),
		data.NewField("loki_log", nil, lokiLogs),
	)

	response.Frames = append(response.Frames, frame)

	return response
}

func (s *Service) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	if req.Path == "status" {
		body, _ := json.Marshal(map[string]string{"status": "ok"})
		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusOK,
			Body:   body,
		})
	}
	return sender.Send(&backend.CallResourceResponse{
		Status: http.StatusNotFound,
	})
}
