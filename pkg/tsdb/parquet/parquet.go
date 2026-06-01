package parquet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type DataSource struct {
	logger log.Logger
}

type DataQuery struct {
	QueryType string        `json:"queryType,omitempty"`
	Paths     []string      `json:"paths,omitempty"`
	Filters   []LabelFilter `json:"filters,omitempty"`
	TimeRange backend.TimeRange
}

type LabelFilter struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}

type ParquetFileInfo struct {
	Path     string
	Columns  []string
	RowCount int64
}

func NewDatasource(_ context.Context, _ backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	ds := &DataSource{
		logger: backend.NewLoggerWith("logger", "grafana-parquet-datasource"),
	}
	return ds, nil
}

func (ds *DataSource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		query, err := ds.parseQuery(q)
		if err != nil {
			response.Responses[q.RefID] = backend.ErrDataResponse(
				backend.StatusBadRequest,
				fmt.Sprintf("Failed to parse query: %v", err),
			)
			continue
		}

		query.TimeRange = q.TimeRange
		frames, err := ds.executeQuery(ctx, query)
		if err != nil {
			response.Responses[q.RefID] = backend.ErrDataResponse(
				backend.StatusInternal,
				fmt.Sprintf("Query execution failed: %v", err),
			)
			continue
		}

		response.Responses[q.RefID] = backend.DataResponse{
			Frames: frames,
		}
	}

	return response, nil
}

func (ds *DataSource) parseQuery(q backend.DataQuery) (*DataQuery, error) {
	var query DataQuery
	if err := json.Unmarshal(q.JSON, &query); err != nil {
		return nil, err
	}
	return &query, nil
}

func (ds *DataSource) executeQuery(ctx context.Context, query *DataQuery) (data.Frames, error) {
	if len(query.Paths) == 0 {
		return nil, fmt.Errorf("no paths provided")
	}

	var allFrames data.Frames
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, path := range query.Paths {
		wg.Add(1)
		go func(p string) {
			defer wg.Done()
			frames, err := ds.readParquetFile(ctx, p, query.Filters)
			if err != nil {
				ds.logger.Error("Failed to read Parquet file", "path", p, "error", err)
				return
			}

			mu.Lock()
			allFrames = append(allFrames, frames...)
			mu.Unlock()
		}(path)
	}

	wg.Wait()
	return allFrames, nil
}

func (ds *DataSource) readParquetFile(_ context.Context, path string, filters []LabelFilter) (data.Frames, error) {
	reader, err := NewParquetReader(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	if err := reader.ApplyFilters(filters); err != nil {
		return nil, err
	}

	return reader.ReadAll()
}

func (ds *DataSource) CheckHealth(_ context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	return &backend.CheckHealthResult{
		Status:  backend.HealthStatusOk,
		Message: "Parquet datasource is healthy",
	}, nil
}

func (ds *DataSource) CallResource(_ context.Context, _ *backend.CallResourceRequest, _ backend.CallResourceResponseSender) error {
	return nil
}
