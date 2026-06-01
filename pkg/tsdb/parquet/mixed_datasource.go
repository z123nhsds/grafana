package parquet

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type MixedDataSource struct {
	datasources map[string]*DataSource
	logger      backend.Logger
}

type MixedQuery struct {
	DatasourceRefs []string        `json:"datasourceRefs,omitempty"`
	Queries        []DataQuery     `json:"queries,omitempty"`
	CombineResults bool            `json:"combineResults,omitempty"`
}

func NewMixedDataSource(logger backend.Logger) *MixedDataSource {
	return &MixedDataSource{
		datasources: make(map[string]*DataSource),
		logger:      logger,
	}
}

func (m *MixedDataSource) RegisterDataSource(name string, ds *DataSource) {
	m.datasources[name] = ds
}

func (m *MixedDataSource) QueryMixedData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	var mixedQueries []MixedQuery
	for _, q := range req.Queries {
		var mixedQuery MixedQuery
		if err := parseJSON(q.JSON, &mixedQuery); err != nil {
			continue
		}
		mixedQueries = append(mixedQueries, mixedQuery)
	}

	if len(mixedQueries) == 0 {
		return response, nil
	}

	var allFrames data.Frames
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, mixedQuery := range mixedQueries {
		for _, ref := range mixedQuery.DatasourceRefs {
			if ds, ok := m.datasources[ref]; ok {
				wg.Add(1)
				go func(d *DataSource, queries []DataQuery) {
					defer wg.Done()
					for _, query := range queries {
						frames, err := m.executeQueryOnDatasource(ctx, d, query, req.Queries[0].TimeRange)
						if err != nil {
							m.logger.Error("Query failed on datasource", "error", err)
							continue
						}
						mu.Lock()
						allFrames = append(allFrames, frames...)
						mu.Unlock()
					}
				}(ds, mixedQuery.Queries)
			}
		}
	}

	wg.Wait()

	if len(allFrames) > 0 {
		if mixedQueries[0].CombineResults {
			combinedFrame, err := combineFrames(allFrames)
			if err == nil {
				allFrames = data.Frames{combinedFrame}
			}
		}
		response.Responses[req.Queries[0].RefID] = backend.DataResponse{
			Frames: allFrames,
		}
	}

	return response, nil
}

func (m *MixedDataSource) executeQueryOnDatasource(ctx context.Context, ds *DataSource, query DataQuery, timeRange backend.TimeRange) (data.Frames, error) {
	query.TimeRange = timeRange
	return ds.executeQuery(ctx, &query)
}

func parseJSON(data []byte, v interface{}) error {
	if len(data) == 0 {
		return fmt.Errorf("empty JSON")
	}
	return json.Unmarshal(data, v)
}

func combineFrames(frames data.Frames) (*data.Frame, error) {
	if len(frames) == 0 {
		return nil, fmt.Errorf("no frames to combine")
	}
	if len(frames) == 1 {
		return frames[0], nil
	}

	combined := &data.Frame{
		Name: "combined_results",
	}

	fieldMap := make(map[string]*data.Field)
	for _, frame := range frames {
		for _, field := range frame.Fields {
			key := field.Name + "_" + string(field.Type)
			if existing, ok := fieldMap[key]; ok {
				combinedField, err := mergeFields(existing, field)
				if err != nil {
					continue
				}
				fieldMap[key] = combinedField
			} else {
				fieldMap[key] = field
			}
		}
	}

	for _, field := range fieldMap {
		combined.Fields = append(combined.Fields, field)
	}

	return combined, nil
}

func mergeFields(a, b *data.Field) (*data.Field, error) {
	if a.Type != b.Type {
		return nil, fmt.Errorf("field type mismatch")
	}

	newField := data.NewField(a.Name, a.Labels, nil)
	newField.Config = a.Config

	switch a.Type {
	case data.FieldTypeTime:
		times := make([]time.Time, 0, a.Len()+b.Len())
		for i := 0; i < a.Len(); i++ {
			t, ok := a.ConcreteAt(i).(time.Time)
			if ok {
				times = append(times, t)
			}
		}
		for i := 0; i < b.Len(); i++ {
			t, ok := b.ConcreteAt(i).(time.Time)
			if ok {
				times = append(times, t)
			}
		}
		newField.Values = times
	case data.FieldTypeString:
		strs := make([]string, 0, a.Len()+b.Len())
		for i := 0; i < a.Len(); i++ {
			if s, ok := a.ConcreteAt(i).(string); ok {
				strs = append(strs, s)
			}
		}
		for i := 0; i < b.Len(); i++ {
			if s, ok := b.ConcreteAt(i).(string); ok {
				strs = append(strs, s)
			}
		}
		newField.Values = strs
	default:
		return nil, fmt.Errorf("unsupported field type: %v", a.Type)
	}

	return newField, nil
}
