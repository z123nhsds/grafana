package mixedquery

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

var (
	_ backend.QueryDataHandler    = (*MixedQueryDatasource)(nil)
	_ backend.CallResourceHandler = (*MixedQueryDatasource)(nil)
	_ backend.CheckHealthHandler  = (*MixedQueryDatasource)(nil)
)

type MixedQueryDatasource struct{}

func NewMixedQueryDatasource(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	return &MixedQueryDatasource{}, nil
}

func (d *MixedQueryDatasource) Dispose() {}

type QueryModel struct {
	QueryType string            `json:"queryType"` // "prometheus", "loki", "mixed"
	PromQL    string            `json:"promql"`
	LokiQL    string            `json:"lokiql"`
	Labels    map[string]string `json:"labels"`
}

func (d *MixedQueryDatasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		var queryModel QueryModel
		if err := json.Unmarshal(q.JSON, &queryModel); err != nil {
			response.Responses[q.RefID] = backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("invalid query: %v", err))
			continue
		}

		frame, err := d.processQuery(ctx, queryModel, q.TimeRange)
		if err != nil {
			response.Responses[q.RefID] = backend.ErrDataResponse(backend.StatusInternal, err.Error())
			continue
		}

		response.Responses[q.RefID] = backend.DataResponse{
			Frames: data.Frames{frame},
		}
	}

	return response, nil
}

func (d *MixedQueryDatasource) processQuery(ctx context.Context, query QueryModel, timeRange backend.TimeRange) (*data.Frame, error) {
	switch query.QueryType {
	case "prometheus":
		return d.generatePrometheusData(query, timeRange)
	case "loki":
		return d.generateLokiData(query, timeRange)
	case "mixed":
		return d.generateMixedData(query, timeRange)
	default:
		return nil, fmt.Errorf("unsupported query type: %s", query.QueryType)
	}
}

func (d *MixedQueryDatasource) generatePrometheusData(query QueryModel, timeRange backend.TimeRange) (*data.Frame, error) {
	frame := data.NewFrame("prometheus_data")
	times := make([]time.Time, 0)
	values := make([]float64, 0)

	current := timeRange.From
	for current.Before(timeRange.To) || current.Equal(timeRange.To) {
		times = append(times, current)
		values = append(values, float64(current.Unix()%100)+50)
		current = current.Add(30 * time.Second)
	}

	frame.Fields = append(frame.Fields,
		data.NewField("Time", nil, times),
		data.NewField("Value", data.Labels(query.Labels), values),
	)

	return frame, nil
}

func (d *MixedQueryDatasource) generateLokiData(query QueryModel, timeRange backend.TimeRange) (*data.Frame, error) {
	frame := data.NewFrame("loki_data")
	times := make([]time.Time, 0)
	messages := make([]string, 0)

	current := timeRange.From
	for current.Before(timeRange.To) || current.Equal(timeRange.To) {
		times = append(times, current)
		messages = append(messages, fmt.Sprintf("Log entry at %s", current.Format(time.RFC3339)))
		current = current.Add(1 * time.Minute)
	}

	frame.Fields = append(frame.Fields,
		data.NewField("Time", nil, times),
		data.NewField("Message", data.Labels(query.Labels), messages),
	)

	return frame, nil
}

type DataPoint struct {
	Time  time.Time
	Value interface{}
	Type  string
}

func (d *MixedQueryDatasource) generateMixedData(query QueryModel, timeRange backend.TimeRange) (*data.Frame, error) {
	frame := data.NewFrame("mixed_data")

	promTimes := make([]time.Time, 0)
	promValues := make([]float64, 0)
	lokiTimes := make([]time.Time, 0)
	lokiMessages := make([]string, 0)

	allPoints := make([]DataPoint, 0)

	current := timeRange.From
	for current.Before(timeRange.To) || current.Equal(timeRange.To) {
		allPoints = append(allPoints, DataPoint{
			Time:  current,
			Value: float64(current.Unix()%100) + 50,
			Type:  "prometheus",
		})
		current = current.Add(30 * time.Second)
	}

	current = timeRange.From
	for current.Before(timeRange.To) || current.Equal(timeRange.To) {
		allPoints = append(allPoints, DataPoint{
			Time:  current,
			Value: fmt.Sprintf("Log entry at %s", current.Format(time.RFC3339)),
			Type:  "loki",
		})
		current = current.Add(1 * time.Minute)
	}

	sort.Slice(allPoints, func(i, j int) bool {
		return allPoints[i].Time.Before(allPoints[j].Time)
	})

	mergedTimes := make([]time.Time, 0, len(allPoints))
	mergedPromValues := make([]*float64, 0, len(allPoints))
	mergedLokiMessages := make([]*string, 0, len(allPoints))

	for _, point := range allPoints {
		mergedTimes = append(mergedTimes, point.Time)
		if point.Type == "prometheus" {
			val := point.Value.(float64)
			mergedPromValues = append(mergedPromValues, &val)
			mergedLokiMessages = append(mergedLokiMessages, nil)
		} else {
			msg := point.Value.(string)
			mergedLokiMessages = append(mergedLokiMessages, &msg)
			mergedPromValues = append(mergedPromValues, nil)
		}
	}

	frame.Fields = append(frame.Fields,
		data.NewField("Time", nil, mergedTimes),
		data.NewField("Prometheus_Value", data.Labels(query.Labels), mergedPromValues),
		data.NewField("Loki_Message", data.Labels(query.Labels), mergedLokiMessages),
	)

	return frame, nil
}

func (d *MixedQueryDatasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	switch req.Path {
	case "health":
		return sender.Send(&backend.CallResourceResponse{
			Status: 200,
			Body:   []byte(`{"status":"ok"}`),
		})
	default:
		return sender.Send(&backend.CallResourceResponse{
			Status: 404,
		})
	}
}

func (d *MixedQueryDatasource) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "Mixed Query Datasource is working",
	}, nil
}
