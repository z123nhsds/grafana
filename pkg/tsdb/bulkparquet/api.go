package bulkparquet

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/grafana/grafana/pkg/api/response"
	"github.com/grafana/grafana/pkg/services/datasources"
	"github.com/grafana/grafana/pkg/util"
	"github.com/grafana/grafana/pkg/util/proxy"
)

type BulkQueryHandler struct {
	dsCache datasources.CacheService
}

func NewBulkQueryHandler(dsCache datasources.CacheService) *BulkQueryHandler {
	return &BulkQueryHandler{
		dsCache: dsCache,
	}
}

type BulkQueryRequest struct {
	Queries []BulkQueryItem `json:"queries"`
}

type BulkQueryItem struct {
	RefID        string     `json:"refId"`
	InputPaths   []string   `json:"inputPaths"`
	LabelFilters []LabelFilterItem `json:"labelFilters"`
	BatchSize    int64      `json:"batchSize"`
}

type LabelFilterItem struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	Operator string `json:"operator"`
}

type BulkQueryResponse struct {
	Data   []BulkResult `json:"data"`
	Errors []BulkError  `json:"errors,omitempty"`
}

type BulkResult struct {
	RefID  string          `json:"refId"`
	Frames []BulkFrame     `json:"frames"`
}

type BulkFrame struct {
	Schema BulkSchema `json:"schema"`
	Data   BulkData   `json:"data"`
}

type BulkSchema struct {
	Name   string       `json:"name,omitempty"`
	Fields []BulkField  `json:"fields"`
}

type BulkField struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Labels Labels `json:"labels,omitempty"`
}

type BulkData struct {
	Values [][]interface{} `json:"values"`
	Length int             `json:"length"`
}

type Labels map[string]string

type BulkError struct {
	RefID   string `json:"refId,omitempty"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}

func (h *BulkQueryHandler) HandleBulkQuery(c *context.Context, w http.ResponseWriter, r *http.Request) response.Response {
	if r.Method != http.MethodPost {
		return response.Error(http.StatusMethodNotAllowed, "method not allowed", nil)
	}

	var req BulkQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return response.Error(http.StatusBadRequest, "invalid request body", err)
	}

	if len(req.Queries) == 0 {
		return response.Error(http.StatusBadRequest, "no queries provided", nil)
	}

	results := make([]BulkResult, 0, len(req.Queries))
	errors := make([]BulkError, 0)

	for _, query := range req.Queries {
		result, err := h.executeQuery(c, query)
		if err != nil {
			errors = append(errors, BulkError{
				RefID:   query.RefID,
				Message: err.Error(),
				Code:    "QUERY_ERROR",
			})
			continue
		}
		results = append(results, BulkResult{
			RefID:  query.RefID,
			Frames: result,
		})
	}

	return response.JSON(http.StatusOK, BulkQueryResponse{
		Data:   results,
		Errors: errors,
	})
}

func (h *BulkQueryHandler) executeQuery(c *context.Context, query BulkQueryItem) ([]BulkFrame, error) {
	if len(query.InputPaths) == 0 {
		return nil, fmt.Errorf("no input paths provided")
	}

	batchSize := query.BatchSize
	if batchSize <= 0 {
		batchSize = 1024
	}

	filters := make([]LabelFilter, 0, len(query.LabelFilters))
	for _, f := range query.LabelFilters {
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

	var reader BulkRequestIterator
	var err error

	if len(query.InputPaths) == 1 {
		reader, err = ReadParquetWithLabels(*c, query.InputPaths[0], filters, batchSize)
	} else {
		reader, err = CreateMixedDataSourceReader(*c, query.InputPaths, filters, batchSize)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create reader: %w", err)
	}

	frames := make([]BulkFrame, 0)
	namespaceValues := make([]interface{}, 0)
	groupValues := make([]interface{}, 0)
	resourceValues := make([]interface{}, 0)
	nameValues := make([]interface{}, 0)
	valueValues := make([]interface{}, 0)
	folderValues := make([]interface{}, 0)
	timestampValues := make([]interface{}, 0)
	labelsValues := make([]interface{}, 0)

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
		timestampValues = append(timestampValues, int64(0))
		labelsValues = append(labelsValues, "{}")
	}

	if reader.RollbackRequested() {
		return nil, fmt.Errorf("rollback requested")
	}

	if len(namespaceValues) > 0 {
		frames = append(frames, BulkFrame{
			Schema: BulkSchema{
				Name: "bulk_requests",
				Fields: []BulkField{
					{Name: "namespace", Type: "string"},
					{Name: "group", Type: "string"},
					{Name: "resource", Type: "string"},
					{Name: "name", Type: "string"},
					{Name: "value", Type: "string"},
					{Name: "folder", Type: "string"},
					{Name: "timestamp", Type: "time"},
					{Name: "labels", Type: "other"},
				},
			},
			Data: BulkData{
				Values: [][]interface{}{
					namespaceValues,
					groupValues,
					resourceValues,
					nameValues,
					valueValues,
					folderValues,
					timestampValues,
					labelsValues,
				},
				Length: len(namespaceValues),
			},
		})
	}

	return frames, nil
}

func (h *BulkQueryHandler) ReverseProxyHandler(w http.ResponseWriter, r *http.Request, ds *datasources.DataSource) {
	proxy.NewProxy(http.Header{}, nil, ds, h.dsCache, h.dsCache, nil, nil).ServeHTTP(w, r)
}

func GenerateUID() string {
	return util.GenerateShortUID()
}