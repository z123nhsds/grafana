package parquet

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDatasource(t *testing.T) {
	ctx := context.Background()
	settings := backend.DataSourceInstanceSettings{}
	ds, err := NewDatasource(ctx, settings)
	
	require.NoError(t, err)
	assert.NotNil(t, ds)
}

func TestCheckHealth(t *testing.T) {
	ctx := context.Background()
	ds, _ := NewDatasource(ctx, backend.DataSourceInstanceSettings{})
	parquetDs := ds.(*DataSource)
	
	result, err := parquetDs.CheckHealth(ctx, &backend.CheckHealthRequest{})
	
	require.NoError(t, err)
	assert.Equal(t, backend.HealthStatusOk, result.Status)
	assert.Equal(t, "Parquet datasource is healthy", result.Message)
}

func TestLabelFilter(t *testing.T) {
	tests := []struct {
		name           string
		entry          LogEntry
		filters        []LabelFilter
		shouldMatch    bool
	}{
		{
			name: "No filters should match everything",
			entry: LogEntry{
				Labels: map[string]string{"env": "prod"},
			},
			filters:     []LabelFilter{},
			shouldMatch: true,
		},
		{
			name: "Equals match",
			entry: LogEntry{
				Labels: map[string]string{"env": "prod"},
			},
			filters: []LabelFilter{
				{
					Key:      "env",
					Operator: "=",
					Values:   []string{"prod"},
				},
			},
			shouldMatch: true,
		},
		{
			name: "Equals no match",
			entry: LogEntry{
				Labels: map[string]string{"env": "staging"},
			},
			filters: []LabelFilter{
				{
					Key:      "env",
					Operator: "=",
					Values:   []string{"prod"},
				},
			},
			shouldMatch: false,
		},
		{
			name: "Not equals match",
			entry: LogEntry{
				Labels: map[string]string{"env": "staging"},
			},
			filters: []LabelFilter{
				{
					Key:      "env",
					Operator: "!=",
					Values:   []string{"prod"},
				},
			},
			shouldMatch: true,
		},
		{
			name: "Multiple values match",
			entry: LogEntry{
				Labels: map[string]string{"env": "staging"},
			},
			filters: []LabelFilter{
				{
					Key:      "env",
					Operator: "=",
					Values:   []string{"prod", "staging"},
				},
			},
			shouldMatch: true,
		},
		{
			name: "Contains match",
			entry: LogEntry{
				Labels: map[string]string{"service": "api-gateway"},
			},
			filters: []LabelFilter{
				{
					Key:      "service",
					Operator: "=~",
					Values:   []string{"api"},
				},
			},
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &ParquetReader{
				filters: tt.filters,
			}
			
			result := reader.matchesFilters(tt.entry)
			assert.Equal(t, tt.shouldMatch, result)
		})
	}
}

func TestReadParquetFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.jsonl")
	
	testData := `{"timestamp": "2024-01-01T00:00:00Z", "level": "info", "message": "Test message 1", "labels": {"env": "prod"}}
{"timestamp": "2024-01-01T00:01:00Z", "level": "error", "message": "Test message 2", "labels": {"env": "staging"}}
`
	require.NoError(t, os.WriteFile(testFile, []byte(testData), 0644))

	reader, err := NewParquetReader(testFile)
	require.NoError(t, err)
	defer reader.Close()

	frames, err := reader.ReadAll()
	require.NoError(t, err)
	assert.Len(t, frames, 1)
	
	frame := frames[0]
	assert.Equal(t, "parquet_logs", frame.Name)
	assert.Greater(t, len(frame.Fields), 0)
}

func TestApplyFilters(t *testing.T) {
	reader := &ParquetReader{}
	filters := []LabelFilter{
		{
			Key:      "env",
			Operator: "=",
			Values:   []string{"prod"},
		},
	}
	
	err := reader.ApplyFilters(filters)
	require.NoError(t, err)
	assert.Equal(t, filters, reader.filters)
}

func TestHasValue(t *testing.T) {
	tests := []struct {
		name    string
		slice   interface{}
		hasVal  bool
	}{
		{
			name:    "Empty slice",
			slice:   []string{},
			hasVal:  false,
		},
		{
			name:    "Slice with value",
			slice:   []string{"test"},
			hasVal:  true,
		},
		{
			name:    "Pointer slice with nil",
			slice:   []*string{nil, nil},
			hasVal:  false,
		},
		{
			name:    "Pointer slice with value",
			slice:   []*string{func(s string) *string { return &s }("test")},
			hasVal:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasValue(tt.slice)
			assert.Equal(t, tt.hasVal, result)
		})
	}
}
