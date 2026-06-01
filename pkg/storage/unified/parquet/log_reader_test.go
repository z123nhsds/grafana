package parquet

import (
	"errors"
	"os"
	"testing"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"
	"github.com/stretchr/testify/require"
)

func logSchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "timestamp", Type: &arrow.Int64Type{}, Nullable: false},
		{Name: "line", Type: &arrow.StringType{}, Nullable: false},
		{Name: "label_key", Type: &arrow.StringType{}, Nullable: false},
		{Name: "label_value", Type: &arrow.StringType{}, Nullable: false},
	}, nil)
}

func writeLogParquetFile(t *testing.T, path string, rows []LogLine) {
	t.Helper()

	f, err := os.Create(path)
	require.NoError(t, err)
	defer func() { _ = f.Close() }()

	pool := memory.DefaultAllocator
	props := parquet.NewWriterProperties(
		parquet.WithCompression(compress.Codecs.Brotli),
	)
	writer, err := pqarrow.NewFileWriter(logSchema(), f, props, pqarrow.DefaultWriterProps())
	require.NoError(t, err)
	defer func() { _ = writer.Close() }()

	tsBuilder := array.NewInt64Builder(pool)
	lineBuilder := array.NewStringBuilder(pool)
	keyBuilder := array.NewStringBuilder(pool)
	valBuilder := array.NewStringBuilder(pool)

	for _, row := range rows {
		tsBuilder.Append(row.Timestamp)
		lineBuilder.Append(row.Line)

		keys := ""
		vals := ""
		first := true
		for k, v := range row.Labels {
			if !first {
				keys += "\x00"
				vals += "\x00"
			}
			keys += k
			vals += v
			first = false
		}
		keyBuilder.Append(keys)
		valBuilder.Append(vals)
	}

	rec := array.NewRecordBatch(logSchema(), []arrow.Array{
		tsBuilder.NewArray(),
		lineBuilder.NewArray(),
		keyBuilder.NewArray(),
		valBuilder.NewArray(),
	}, int64(len(rows)))
	defer rec.Release()

	require.NoError(t, writer.Write(rec))
	require.NoError(t, writer.Close())
}

func TestLogParquetReaderReadAll(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "log-*.parquet")
	require.NoError(t, err)
	path := file.Name()
	_ = file.Close()

	writeLogParquetFile(t, path, []LogLine{
		{Timestamp: 1000, Line: "first log line", Labels: map[string]string{"app": "nginx", "env": "prod"}},
		{Timestamp: 2000, Line: "second log line", Labels: map[string]string{"app": "nginx", "env": "staging"}},
		{Timestamp: 3000, Line: "third log line", Labels: map[string]string{"app": "api", "env": "prod"}},
	})

	reader, err := NewLogParquetReader(path, 20, nil)
	require.NoError(t, err)
	defer reader.Close()

	var lines []LogLine
	for reader.Next() {
		lines = append(lines, *reader.Line())
	}
	require.NoError(t, reader.Err())

	require.Len(t, lines, 3)
	require.Equal(t, "first log line", lines[0].Line)
	require.Equal(t, "second log line", lines[1].Line)
	require.Equal(t, "third log line", lines[2].Line)
}

func TestLogParquetReaderWithLabelFilter(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "log-*.parquet")
	require.NoError(t, err)
	path := file.Name()
	_ = file.Close()

	writeLogParquetFile(t, path, []LogLine{
		{Timestamp: 1000, Line: "log A", Labels: map[string]string{"app": "nginx", "env": "prod"}},
		{Timestamp: 2000, Line: "log B", Labels: map[string]string{"app": "nginx", "env": "staging"}},
		{Timestamp: 3000, Line: "log C", Labels: map[string]string{"app": "api", "env": "prod"}},
		{Timestamp: 4000, Line: "log D", Labels: map[string]string{"app": "api", "env": "staging"}},
	})

	t.Run("filter by single label equal", func(t *testing.T) {
		reader, err := NewLogParquetReader(path, 20, []LabelFilter{
			{Key: "app", Value: "nginx", Op: LabelFilterOpEqual},
		})
		require.NoError(t, err)
		defer reader.Close()

		var lines []LogLine
		for reader.Next() {
			lines = append(lines, *reader.Line())
		}
		require.NoError(t, reader.Err())
		require.Len(t, lines, 2)
		require.Equal(t, "log A", lines[0].Line)
		require.Equal(t, "log B", lines[1].Line)
	})

	t.Run("filter by multiple labels", func(t *testing.T) {
		reader, err := NewLogParquetReader(path, 20, []LabelFilter{
			{Key: "app", Value: "nginx", Op: LabelFilterOpEqual},
			{Key: "env", Value: "prod", Op: LabelFilterOpEqual},
		})
		require.NoError(t, err)
		defer reader.Close()

		var lines []LogLine
		for reader.Next() {
			lines = append(lines, *reader.Line())
		}
		require.NoError(t, reader.Err())
		require.Len(t, lines, 1)
		require.Equal(t, "log A", lines[0].Line)
	})

	t.Run("filter by not equal", func(t *testing.T) {
		reader, err := NewLogParquetReader(path, 20, []LabelFilter{
			{Key: "env", Value: "prod", Op: LabelFilterOpNotEqual},
		})
		require.NoError(t, err)
		defer reader.Close()

		var lines []LogLine
		for reader.Next() {
			lines = append(lines, *reader.Line())
		}
		require.NoError(t, reader.Err())
		require.Len(t, lines, 2)
	})
}

func TestLogParquetReaderEmptyFile(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "log-*.parquet")
	require.NoError(t, err)
	path := file.Name()
	_ = file.Close()

	writeLogParquetFile(t, path, nil)

	reader, err := NewLogParquetReader(path, 20, nil)
	require.NoError(t, err)
	defer reader.Close()

	var lines []LogLine
	for reader.Next() {
		lines = append(lines, *reader.Line())
	}
	require.NoError(t, reader.Err())
	require.Empty(t, lines)
}

func TestLogParquetReaderDisablesMemoryMap(t *testing.T) {
	originalOpenParquetFile := openParquetFile
	t.Cleanup(func() {
		openParquetFile = originalOpenParquetFile
	})

	openParquetFile = func(_ string, memoryMap bool) (*file.Reader, error) {
		require.False(t, memoryMap)
		return nil, errors.New("boom")
	}

	reader, err := NewLogParquetReader("ignored.parquet", 20, nil)
	require.Nil(t, reader)
	require.EqualError(t, err, "boom")
}

func TestParseLabels(t *testing.T) {
	t.Run("empty strings", func(t *testing.T) {
		labels := parseLabels("", "")
		require.Empty(t, labels)
	})

	t.Run("single label", func(t *testing.T) {
		labels := parseLabels("app", "nginx")
		require.Len(t, labels, 1)
		require.Equal(t, "nginx", labels["app"])
	})

	t.Run("multiple labels", func(t *testing.T) {
		labels := parseLabels("app\x00env", "nginx\x00prod")
		require.Len(t, labels, 2)
		require.Equal(t, "nginx", labels["app"])
		require.Equal(t, "prod", labels["env"])
	})

	t.Run("mismatched lengths", func(t *testing.T) {
		labels := parseLabels("app\x00env", "nginx")
		require.Len(t, labels, 1)
		require.Equal(t, "nginx", labels["app"])
	})
}

func TestMatchesFilters(t *testing.T) {
	labels := map[string]string{"app": "nginx", "env": "prod"}

	t.Run("no filters always matches", func(t *testing.T) {
		r := &LogParquetReader{}
		require.True(t, r.matchesFilters(labels))
	})

	t.Run("equal filter matches", func(t *testing.T) {
		r := &LogParquetReader{
			filters: []LabelFilter{{Key: "app", Value: "nginx", Op: LabelFilterOpEqual}},
		}
		require.True(t, r.matchesFilters(labels))
	})

	t.Run("equal filter does not match wrong value", func(t *testing.T) {
		r := &LogParquetReader{
			filters: []LabelFilter{{Key: "app", Value: "api", Op: LabelFilterOpEqual}},
		}
		require.False(t, r.matchesFilters(labels))
	})

	t.Run("equal filter does not match missing key", func(t *testing.T) {
		r := &LogParquetReader{
			filters: []LabelFilter{{Key: "missing", Value: "val", Op: LabelFilterOpEqual}},
		}
		require.False(t, r.matchesFilters(labels))
	})

	t.Run("not equal filter matches", func(t *testing.T) {
		r := &LogParquetReader{
			filters: []LabelFilter{{Key: "app", Value: "api", Op: LabelFilterOpNotEqual}},
		}
		require.True(t, r.matchesFilters(labels))
	})

	t.Run("not equal filter does not match same value", func(t *testing.T) {
		r := &LogParquetReader{
			filters: []LabelFilter{{Key: "app", Value: "nginx", Op: LabelFilterOpNotEqual}},
		}
		require.False(t, r.matchesFilters(labels))
	})

	t.Run("not equal filter matches missing key", func(t *testing.T) {
		r := &LogParquetReader{
			filters: []LabelFilter{{Key: "missing", Value: "val", Op: LabelFilterOpNotEqual}},
		}
		require.True(t, r.matchesFilters(labels))
	})
}