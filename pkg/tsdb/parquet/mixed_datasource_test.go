package parquet

import (
	"context"
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewMixedDataSource(t *testing.T) {
	logger := log.NewNoop()
	ds := NewMixedDataSource(logger)
	
	require.NotNil(t, ds)
	assert.NotNil(t, ds.datasources)
}

func TestRegisterDataSource(t *testing.T) {
	logger := log.NewNoop()
	mixed := NewMixedDataSource(logger)
	
	ctx := context.Background()
	ds, _ := NewDatasource(ctx, backend.DataSourceInstanceSettings{})
	parquetDs := ds.(*DataSource)
	
	mixed.RegisterDataSource("parquet1", parquetDs)
	mixed.RegisterDataSource("parquet2", parquetDs)
	
	assert.Equal(t, 2, len(mixed.datasources))
	assert.Contains(t, mixed.datasources, "parquet1")
	assert.Contains(t, mixed.datasources, "parquet2")
}

func TestParseJSON(t *testing.T) {
	tests := []struct {
		name        string
		input       []byte
		expectErr   bool
	}{
		{
			name:        "Empty JSON should error",
			input:       []byte{},
			expectErr:   true,
		},
		{
			name:        "Valid JSON",
			input:       []byte(`{"datasourceRefs": ["test"], "queries": []}`),
			expectErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result MixedQuery
			err := parseJSON(tt.input, &result)
			
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
