package bulkparquet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

type Service struct {
	im     instancemgmt.InstanceManager
	logger log.Logger
}

var (
	_ backend.QueryDataHandler = (*Service)(nil)
)

func ProvideService() *Service {
	logger := backend.NewLoggerWith("logger", "tsdb.bulkparquet")
	return &Service{
		im:     datasource.NewInstanceManager(newInstanceSettings(logger)),
		logger: logger,
	}
}

type datasourceInfo struct {
	inputPath  string
	batchSize  int64
}

func newInstanceSettings(logger log.Logger) datasource.InstanceFactoryFunc {
	return func(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
		inputPath := ""
		if settings.JSONData != nil {
			if v, ok := settings.JSONData["inputPath"]; ok {
				if s, ok := v.(string); ok {
					inputPath = s
				}
			}
		}

		batchSize := int64(1024)
		if settings.JSONData != nil {
			if v, ok := settings.JSONData["batchSize"]; ok {
				if f, ok := v.(float64); ok {
					batchSize = int64(f)
				}
			}
		}

		model := &datasourceInfo{
			inputPath: inputPath,
			batchSize: batchSize,
		}
		return model, nil
	}
}

func (s *Service) getDSInfo(ctx context.Context, pluginCtx backend.PluginContext) (*datasourceInfo, error) {
	instance, err := s.im.Get(ctx, pluginCtx)
	if err != nil {
		return nil, err
	}
	dsInfo, ok := instance.(*datasourceInfo)
	if !ok {
		return nil, fmt.Errorf("invalid datasource info type")
	}
	return dsInfo, nil
}

func (s *Service) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	dsInfo, err := s.getDSInfo(ctx, req.PluginContext)
	if err != nil {
		s.logger.Error("Failed to get datasource info", "error", err)
		return nil, err
	}

	result := backend.NewQueryDataResponse()

	for _, query := range req.Queries {
		queryRes, err := s.executeQuery(ctx, query, dsInfo)
		if err != nil {
			result.Responses[query.RefID] = backend.DataResponse{
				Error: fmt.Errorf("query execution failed: %w", err),
			}
			continue
		}
		result.Responses[query.RefID] = backend.DataResponse{
			Frames: queryRes,
		}
	}

	return result, nil
}

func (s *Service) executeQuery(ctx context.Context, query backend.DataQuery, dsInfo *datasourceInfo) (backend.Frames, error) {
	var inputPaths []string
	var filters []LabelFilter
	var batchSize int64 = dsInfo.batchSize

	if query.JSON != nil {
		var queryModel QueryJSONModel
		if err := json.Unmarshal(query.JSON, &queryModel); err == nil {
			if len(queryModel.InputPaths) > 0 {
				inputPaths = queryModel.InputPaths
			}
			if queryModel.BatchSize > 0 {
				batchSize = queryModel.BatchSize
			}
			for _, f := range queryModel.LabelFilters {
				op := LabelOpEqual
				switch f.Operator {
				case "=":
					op = LabelOpEqual
				case "!=":
					op = LabelOpNotEqual
				case "=~":
					op = LabelOpRegexMatch
				case "!=~":
					op = LabelOpRegexNotMatch
				}
				filters = append(filters, LabelFilter{
					Key:   f.Key,
					Value: f.Value,
					Op:    op,
				})
			}
		}
	}

	if len(inputPaths) == 0 && dsInfo.inputPath != "" {
		inputPaths = []string{dsInfo.inputPath}
	}

	if len(inputPaths) == 0 {
		return nil, fmt.Errorf("no input path configured")
	}

	var reader BulkRequestIterator
	var readerErr error

	if len(inputPaths) == 1 {
		reader, readerErr = ReadParquetWithLabels(ctx, inputPaths[0], filters, batchSize)
	} else {
		reader, readerErr = CreateMixedDataSourceReader(ctx, inputPaths, filters, batchSize)
	}

	if readerErr != nil {
		return nil, fmt.Errorf("failed to create reader: %w", readerErr)
	}

	frames := make(backend.Frames, 0)

	namespaceValues := make([]string, 0)
	groupValues := make([]string, 0)
	resourceValues := make([]string, 0)
	nameValues := make([]string, 0)
	valueValues := make([]string, 0)
	folderValues := make([]string, 0)
	labelsValues := make([]string, 0)

	for reader.Next() {
		req := reader.Request()
		if req == nil {
			continue
		}

		namespaceValues = append(namespaceValues, req.Key.Namespace)
		groupValues = append(groupValues, req.Key.Group)
		resourceValues = append(resourceValues, req.Key.Resource)
		nameValues = append(nameValues, req.Key.Name)
		valueValues = append(valueValues, string(req.Value))
		folderValues = append(folderValues, req.Folder)
		labelsValues = append(labelsValues, "{}")
	}

	if reader.RollbackRequested() {
		return nil, fmt.Errorf("rollback requested")
	}

	if len(namespaceValues) > 0 {
		frame := backend.NewFrame("bulk_requests")
		frame.Fields = append(frame.Fields,
			backend.NewField("namespace", nil, namespaceValues),
			backend.NewField("group", nil, groupValues),
			backend.NewField("resource", nil, resourceValues),
			backend.NewField("name", nil, nameValues),
			backend.NewField("value", nil, valueValues),
			backend.NewField("folder", nil, folderValues),
			backend.NewField("labels", nil, labelsValues),
		)
		frames = append(frames, frame)
	}

	return frames, nil
}

type QueryJSONModel struct {
	InputPaths   []string        `json:"inputPaths"`
	LabelFilters []LabelFilterJSON `json:"labelFilters"`
	BatchSize    int64           `json:"batchSize"`
}

type LabelFilterJSON struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Operator string `json:"operator"`
}