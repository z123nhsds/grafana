package bulkparquet

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/grafana/grafana/pkg/storage/unified/resourcepb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLabelOperatorString(t *testing.T) {
	tests := []struct {
		op     LabelOperator
		expect string
	}{
		{LabelOpEqual, "="},
		{LabelOpNotEqual, "!="},
		{LabelOpRegexMatch, "=~"},
		{LabelOpRegexNotMatch, "!=~"},
		{LabelOperator(100), "="},
	}

	for _, tt := range tests {
		t.Run(tt.expect, func(t *testing.T) {
			assert.Equal(t, tt.expect, tt.op.String())
		})
	}
}

func TestFindLabelValue(t *testing.T) {
	tests := []struct {
		name     string
		labels   string
		key      string
		startIdx int
		expect   int
	}{
		{
			name:     "found at start",
			labels:   `{"app":"grafana"}`,
			key:      `app":"`,
			startIdx: 0,
			expect:   1,
		},
		{
			name:     "not found",
			labels:   `{"app":"grafana"}`,
			key:      `foo":"`,
			startIdx: 0,
			expect:   -1,
		},
		{
			name:     "found after start index",
			labels:   `{"app":"grafana","env":"prod"}`,
			key:      `env":"`,
			startIdx: 0,
			expect:   18,
		},
		{
			name:     "start index beyond match",
			labels:   `{"app":"grafana","env":"prod"}`,
			key:      `app":"`,
			startIdx: 5,
			expect:   -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLabelValue(tt.labels, tt.key, tt.startIdx)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestFindLabelEnd(t *testing.T) {
	tests := []struct {
		name     string
		labels   string
		start    int
		expected int
	}{
		{
			name:     "simple value",
			labels:   `{"app":"grafana"}`,
			start:    7,
			expected: 14,
		},
		{
			name:     "value with comma",
			labels:   `{"app":"grafana","env":"prod"}`,
			start:    7,
			expected: 14,
		},
		{
			name:     "no closing quote",
			labels:   `{"app":"grafana`,
			start:    7,
			expected: -1,
		},
		{
			name:     "escaped quote",
			labels:   `{"app":"gra\"fana"}`,
			start:    7,
			expected: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findLabelEnd(tt.labels, tt.start)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchLabel(t *testing.T) {
	tests := []struct {
		name   string
		labels string
		filter LabelFilter
		expect bool
	}{
		{
			name:   "equal match",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "app", Value: "grafana", Op: LabelOpEqual},
			expect: true,
		},
		{
			name:   "equal no match",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "app", Value: "prometheus", Op: LabelOpEqual},
			expect: false,
		},
		{
			name:   "not equal match (value differs)",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "app", Value: "prometheus", Op: LabelOpNotEqual},
			expect: true,
		},
		{
			name:   "not equal no match (same value)",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "app", Value: "grafana", Op: LabelOpNotEqual},
			expect: false,
		},
		{
			name:   "regex match",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "app", Value: "graf.*", Op: LabelOpRegexMatch},
			expect: true,
		},
		{
			name:   "regex no match",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "app", Value: "prom.*", Op: LabelOpRegexMatch},
			expect: false,
		},
		{
			name:   "regex not match match",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "app", Value: "prom.*", Op: LabelOpRegexNotMatch},
			expect: true,
		},
		{
			name:   "key not found with equal op",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "env", Value: "prod", Op: LabelOpEqual},
			expect: false,
		},
		{
			name:   "key not found with not equal op",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "env", Value: "prod", Op: LabelOpNotEqual},
			expect: true,
		},
		{
			name:   "key not found with regex not match op",
			labels: `{"app":"grafana"}`,
			filter: LabelFilter{Key: "env", Value: ".*", Op: LabelOpRegexNotMatch},
			expect: true,
		},
		{
			name:   "multiple labels - first matches",
			labels: `{"app":"grafana","env":"prod"}`,
			filter: LabelFilter{Key: "app", Value: "grafana", Op: LabelOpEqual},
			expect: true,
		},
		{
			name:   "multiple labels - second matches",
			labels: `{"app":"grafana","env":"prod"}`,
			filter: LabelFilter{Key: "env", Value: "prod", Op: LabelOpEqual},
			expect: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchLabel(tt.labels, tt.filter)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func TestMatchRegexSimple(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		pattern string
		expect  bool
	}{
		{
			name:    "exact match",
			value:   "grafana",
			pattern: "grafana",
			expect:  true,
		},
		{
			name:    "no match",
			value:   "grafana",
			pattern: "prometheus",
			expect:  false,
		},
		{
			name:    "empty pattern matches empty value",
			value:   "",
			pattern: "",
			expect:  true,
		},
		{
			name:    "empty pattern no match non-empty value",
			value:   "grafana",
			pattern: "",
			expect:  false,
		},
		{
			name:    "wildcard match",
			value:   "grafana",
			pattern: ".*",
			expect:  true,
		},
		{
			name:    "prefix match",
			value:   "grafana",
			pattern: "graf.*",
			expect:  true,
		},
		{
			name:    "suffix match",
			value:   "grafana",
			pattern: ".*na",
			expect:  true,
		},
		{
			name:    "start anchor",
			value:   "grafana",
			pattern: "^graf",
			expect:  true,
		},
		{
			name:    "end anchor",
			value:   "grafana",
			pattern: "ana$",
			expect:  true,
		},
		{
			name:    "combined anchors",
			value:   "grafana",
			pattern: "^grafana$",
			expect:  true,
		},
		{
			name:    "dot match any",
			value:   "grafana",
			pattern: "gr.f.n.",
			expect:  true,
		},
		{
			name:    "plus one or more",
			value:   "graffana",
			pattern: "graf+ana",
			expect:  true,
		},
		{
			name:    "question zero or one",
			value:   "grafana",
			pattern: "graf?ana",
			expect:  false,
		},
		{
			name:    "character class",
			value:   "grafana",
			pattern: "gr[aeiou]f[aeiou]n[aeiou]",
			expect:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := matchRegexSimple(tt.value, tt.pattern)
			require.NoError(t, err)
			assert.Equal(t, tt.expect, result)
		})
	}
}

func createTestParquetFile(t *testing.T, path string, rows []testRow) {
	t.Helper()

	writer, err := NewWriter(path, WriteOptions{RowGroupSize: 100})
	require.NoError(t, err)

	for _, row := range rows {
		key := &resourcepb.ResourceKey{
			Namespace: row.Namespace,
			Group:     row.Group,
			Resource:  row.Resource,
			Name:     row.Name,
		}
		err := writer.Write(context.Background(), key, []byte(row.Value), row.Labels)
		require.NoError(t, err)
	}

	_, err = writer.CloseWithResults()
	require.NoError(t, err)
}

type testRow struct {
	Namespace string
	Group     string
	Resource  string
	Name      string
	Value     string
	Folder    string
	Action    int32
	Timestamp int64
	Labels    string
}

func TestParquetReaderBasic(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"prometheus"}`},
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"loki"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		req := reader.Request()
		require.NotNil(t, req)
		require.NotNil(t, req.Key)
		count++
	}
	assert.Equal(t, 3, count)
	assert.False(t, reader.RollbackRequested())
}

func TestParquetReaderWithLabelFilters(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"prometheus"}`},
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"grafana"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
		LabelFilters: []LabelFilter{
			{Key: "app", Value: "grafana", Op: LabelOpEqual},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		req := reader.Request()
		require.NotNil(t, req)
		require.NotNil(t, req.Key)
		count++
	}
	assert.Equal(t, 2, count)
}

func TestParquetReaderWithMultipleLabelFilters(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana","env":"prod"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"grafana","env":"dev"}`},
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"prometheus","env":"prod"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
		LabelFilters: []LabelFilter{
			{Key: "app", Value: "grafana", Op: LabelOpEqual},
			{Key: "env", Value: "prod", Op: LabelOpEqual},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		req := reader.Request()
		require.NotNil(t, req)
		count++
	}
	assert.Equal(t, 1, count)
}

func TestParquetReaderRegexLabelFilter(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana-prod"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"grafana-dev"}`},
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"prometheus"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
		LabelFilters: []LabelFilter{
			{Key: "app", Value: "grafana-.*", Op: LabelOpRegexMatch},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		req := reader.Request()
		require.NotNil(t, req)
		count++
	}
	assert.Equal(t, 2, count)
}

func TestParquetReaderEmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "empty.parquet")

	rows := []testRow{}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	assert.False(t, reader.Next())
	assert.Nil(t, reader.Request())
	assert.False(t, reader.RollbackRequested())
}

func TestParquetReaderNonexistentFile(t *testing.T) {
	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: "/nonexistent/path/file.parquet",
		BatchSize: 10,
	})
	require.Error(t, err)
	require.Nil(t, reader)
	assert.Contains(t, err.Error(), "failed to open parquet file")
}

func TestParquetReaderDefaultBatchSize(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 0,
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	assert.True(t, reader.Next())
	req := reader.Request()
	require.NotNil(t, req)
	assert.Equal(t, "default", req.Key.Namespace)
}

func TestMixedParquetReader(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath1 := filepath.Join(tmpDir, "test1.parquet")
	parquetPath2 := filepath.Join(tmpDir, "test2.parquet")

	rows1 := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"loki"}`},
	}
	rows2 := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"prometheus"}`},
		{Namespace: "default", Group: "group1", Resource: "res4", Name: "name4", Value: "value4", Labels: `{"app":"tempo"}`},
	}

	createTestParquetFile(t, parquetPath1, rows1)
	createTestParquetFile(t, parquetPath2, rows2)

	ctx := context.Background()
	reader, err := CreateMixedDataSourceReader(ctx, []string{parquetPath1, parquetPath2}, nil, 10)
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		req := reader.Request()
		require.NotNil(t, req)
		count++
	}
	assert.Equal(t, 4, count)
	assert.False(t, reader.RollbackRequested())
}

func TestMixedParquetReaderWithFilters(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath1 := filepath.Join(tmpDir, "test1.parquet")
	parquetPath2 := filepath.Join(tmpDir, "test2.parquet")

	rows1 := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana","env":"prod"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"loki","env":"dev"}`},
	}
	rows2 := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"grafana","env":"prod"}`},
		{Namespace: "default", Group: "group1", Resource: "res4", Name: "name4", Value: "value4", Labels: `{"app":"tempo","env":"prod"}`},
	}

	createTestParquetFile(t, parquetPath1, rows1)
	createTestParquetFile(t, parquetPath2, rows2)

	ctx := context.Background()
	reader, err := CreateMixedDataSourceReader(ctx, []string{parquetPath1, parquetPath2}, []LabelFilter{
		{Key: "app", Value: "grafana", Op: LabelOpEqual},
	}, 10)
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		req := reader.Request()
		require.NotNil(t, req)
		count++
	}
	assert.Equal(t, 2, count)
}

func TestMixedParquetReaderPartialError(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath1 := filepath.Join(tmpDir, "test1.parquet")
	parquetPath2 := filepath.Join(tmpDir, "test2.parquet")
	parquetPath3 := "/nonexistent/path.parquet"

	rows1 := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
	}
	rows2 := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"loki"}`},
	}

	createTestParquetFile(t, parquetPath1, rows1)
	createTestParquetFile(t, parquetPath2, rows2)

	ctx := context.Background()
	reader, err := CreateMixedDataSourceReader(ctx, []string{parquetPath1, parquetPath3, parquetPath2}, nil, 10)
	require.Error(t, err)
	require.Nil(t, reader)
	assert.Contains(t, err.Error(), "failed to create reader")
}

func TestReadParquetWithLabels(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"prometheus"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := ReadParquetWithLabels(ctx, parquetPath, []LabelFilter{
		{Key: "app", Value: "grafana", Op: LabelOpEqual},
	}, 10)
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		count++
	}
	assert.Equal(t, 1, count)
}

func TestParquetReaderMatchesLabelFilters(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana","env":"prod"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"prometheus","env":"prod"}`},
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"grafana","env":"dev"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
		LabelFilters: []LabelFilter{
			{Key: "env", Value: "prod", Op: LabelOpEqual},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

func TestParquetReaderNotEqualLabelFilter(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
		{Namespace: "default", Group: "group1", Resource: "res2", Name: "name2", Value: "value2", Labels: `{"app":"prometheus"}`},
		{Namespace: "default", Group: "group1", Resource: "res3", Name: "name3", Value: "value3", Labels: `{"app":"loki"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
		LabelFilters: []LabelFilter{
			{Key: "app", Value: "grafana", Op: LabelOpNotEqual},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, reader)

	count := 0
	for reader.Next() {
		count++
	}
	assert.Equal(t, 2, count)
}

func TestBulkRequestIteratorInterface(t *testing.T) {
	var iterator BulkRequestIterator = nil
	assert.Nil(t, iterator)
}

func TestBulkRequestKeyValues(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{
			Namespace: "test-namespace",
			Group:     "test-group",
			Resource:  "test-resource",
			Name:      "test-name",
			Value:     "test-value",
			Folder:    "test-folder",
			Action:    int32(resourcepb.BulkRequest_WATCH),
			Timestamp: 1234567890,
			Labels:    `{"app":"grafana","env":"test"}`,
		},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
	})
	require.NoError(t, err)

	assert.True(t, reader.Next())
	req := reader.Request()
	require.NotNil(t, req)
	require.NotNil(t, req.Key)

	assert.Equal(t, "test-namespace", req.Key.Namespace)
	assert.Equal(t, "test-group", req.Key.Group)
	assert.Equal(t, "test-resource", req.Key.Resource)
	assert.Equal(t, "test-name", req.Key.Name)
	assert.Equal(t, "test-folder", req.Folder)
	assert.Equal(t, resourcepb.BulkRequest_WATCH, req.Action)
	assert.Equal(t, []byte("test-value"), req.Value)
}

func TestMixedParquetReaderClose(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	reader, err := CreateMixedDataSourceReader(ctx, []string{parquetPath}, nil, 10)
	require.NoError(t, err)

	reader.Next()
	reader.Next()

	err = reader.Close()
	assert.NoError(t, err)
}

func TestParquetReaderClose(t *testing.T) {
	tmpDir := t.TempDir()
	parquetPath := filepath.Join(tmpDir, "test.parquet")

	rows := []testRow{
		{Namespace: "default", Group: "group1", Resource: "res1", Name: "name1", Value: "value1", Labels: `{"app":"grafana"}`},
	}
	createTestParquetFile(t, parquetPath, rows)

	ctx := context.Background()
	preader, err := NewParquetReader(ctx, ParquetReaderConfig{
		InputPath: parquetPath,
		BatchSize: 10,
	})
	require.NoError(t, err)

	if closer, ok := preader.(ioCloser); ok {
		err = closer.Close()
		assert.NoError(t, err)
	}
}

func TestMixedParquetReaderEmptyPaths(t *testing.T) {
	ctx := context.Background()
	reader, err := CreateMixedDataSourceReader(ctx, []string{}, nil, 10)
	require.NoError(t, err)
	require.NotNil(t, reader)

	assert.False(t, reader.Next())
	assert.Nil(t, reader.Request())
}