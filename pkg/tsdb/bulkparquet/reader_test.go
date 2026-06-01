package bulkparquet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/grafana/grafana/pkg/apimachinery/utils"
	"github.com/grafana/grafana/pkg/storage/unified/resourcepb"
)

func TestNewParquetReader(t *testing.T) {
	t.Run("creates reader with default batch size", func(t *testing.T) {
		tmpDir := t.TempDir()
		parquetFile := filepath.Join(tmpDir, "test.parquet")

		writer, err := NewWriter(parquetFile, WriteOptions{RowGroupSize: 100})
		require.NoError(t, err)

		writer.Write(context.Background(), &resourcepb.ResourceKey{
			Namespace: "ns1",
			Group:    "group1",
			Resource: "res1",
			Name:     "name1",
		}, []byte(`{"test":"data"}`), `{"env":"test"}`)

		_, err = writer.CloseWithResults()
		require.NoError(t, err)

		reader, err := NewParquetReader(context.Background(), ParquetReaderConfig{
			InputPath: parquetFile,
			BatchSize: 1024,
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
		require.Equal(t, 1, count)
	})

	t.Run("returns error for non-existent file", func(t *testing.T) {
		reader, err := NewParquetReader(context.Background(), ParquetReaderConfig{
			InputPath: "/non/existent/file.parquet",
			BatchSize: 1024,
		})
		require.Error(t, err)
		require.Nil(t, reader)
	})
}

func TestParquetReaderWithLabels(t *testing.T) {
	tmpDir := t.TempDir()
	parquetFile := filepath.Join(tmpDir, "test_labels.parquet")

	writer, err := NewWriter(parquetFile, WriteOptions{RowGroupSize: 100})
	require.NoError(t, err)

	testCases := []struct {
		name      string
		namespace string
		group     string
		resource  string
		labels    string
	}{
		{"item1", "ns1", "group1", "res1", `{"env":"prod","service":"api"}`},
		{"item2", "ns1", "group1", "res1", `{"env":"dev","service":"web"}`},
		{"item3", "ns2", "group2", "res2", `{"env":"prod","service":"worker"}`},
		{"item4", "ns2", "group2", "res2", `{"env":"staging","service":"api"}`},
	}

	for _, tc := range testCases {
		writer.Write(context.Background(), &resourcepb.ResourceKey{
			Namespace: tc.namespace,
			Group:     tc.group,
			Resource:  tc.resource,
			Name:      tc.name,
		}, []byte(`{"data":"value"}`), tc.labels)
	}

	_, err = writer.CloseWithResults()
	require.NoError(t, err)

	t.Run("filters by exact label match", func(t *testing.T) {
		filters := []LabelFilter{
			{Key: "env", Value: "prod", Op: LabelOpEqual},
		}

		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, filters, 1024)
		require.NoError(t, err)

		count := 0
		for reader.Next() {
			req := reader.Request()
			assert.NotNil(t, req)
			count++
		}
		assert.Equal(t, 2, count)
	})

	t.Run("filters by label inequality", func(t *testing.T) {
		filters := []LabelFilter{
			{Key: "env", Value: "prod", Op: LabelOpNotEqual},
		}

		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, filters, 1024)
		require.NoError(t, err)

		count := 0
		for reader.Next() {
			req := reader.Request()
			assert.NotNil(t, req)
			count++
		}
		assert.Equal(t, 2, count)
	})

	t.Run("filters by regex match", func(t *testing.T) {
		filters := []LabelFilter{
			{Key: "service", Value: "api|web", Op: LabelOpRegexMatch},
		}

		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, filters, 1024)
		require.NoError(t, err)

		count := 0
		for reader.Next() {
			req := reader.Request()
			assert.NotNil(t, req)
			count++
		}
		assert.Equal(t, 3, count)
	})

	t.Run("filters by multiple labels", func(t *testing.T) {
		filters := []LabelFilter{
			{Key: "env", Value: "prod", Op: LabelOpEqual},
			{Key: "service", Value: "api", Op: LabelOpEqual},
		}

		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, filters, 1024)
		require.NoError(t, err)

		count := 0
		for reader.Next() {
			req := reader.Request()
			assert.NotNil(t, req)
			count++
		}
		assert.Equal(t, 1, count)
	})

	t.Run("no filters returns all", func(t *testing.T) {
		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, nil, 1024)
		require.NoError(t, err)

		count := 0
		for reader.Next() {
			req := reader.Request()
			assert.NotNil(t, req)
			count++
		}
		assert.Equal(t, 4, count)
	})
}

func TestMixedParquetReader(t *testing.T) {
	tmpDir := t.TempDir()

	parquetFile1 := filepath.Join(tmpDir, "test1.parquet")
	writer1, err := NewWriter(parquetFile1, WriteOptions{RowGroupSize: 100})
	require.NoError(t, err)

	writer1.Write(context.Background(), &resourcepb.ResourceKey{
		Namespace: "ns1", Group: "g1", Resource: "r1", Name: "n1",
	}, []byte(`{"source":1}`), `{"env":"test"}`)
	writer1.Write(context.Background(), &resourcepb.ResourceKey{
		Namespace: "ns1", Group: "g1", Resource: "r1", Name: "n2",
	}, []byte(`{"source":2}`), `{"env":"test"}`)
	_, err = writer1.CloseWithResults()
	require.NoError(t, err)

	parquetFile2 := filepath.Join(tmpDir, "test2.parquet")
	writer2, err := NewWriter(parquetFile2, WriteOptions{RowGroupSize: 100})
	require.NoError(t, err)

	writer2.Write(context.Background(), &resourcepb.ResourceKey{
		Namespace: "ns2", Group: "g2", Resource: "r2", Name: "n3",
	}, []byte(`{"source":3}`), `{"env":"prod"}`)
	writer2.Write(context.Background(), &resourcepb.ResourceKey{
		Namespace: "ns2", Group: "g2", Resource: "r2", Name: "n4",
	}, []byte(`{"source":4}`), `{"env":"prod"}`)
	writer2.Write(context.Background(), &resourcepb.ResourceKey{
		Namespace: "ns2", Group: "g2", Resource: "r2", Name: "n5",
	}, []byte(`{"source":5}`), `{"env":"dev"}`)
	_, err = writer2.CloseWithResults()
	require.NoError(t, err)

	t.Run("reads from multiple parquet files", func(t *testing.T) {
		filters := []LabelFilter{
			{Key: "env", Value: "prod", Op: LabelOpEqual},
		}

		mixedReader, err := CreateMixedDataSourceReader(context.Background(), []string{parquetFile1, parquetFile2}, filters, 1024)
		require.NoError(t, err)

		count := 0
		seenNS := make(map[string]bool)
		for mixedReader.Next() {
			req := mixedReader.Request()
			require.NotNil(t, req)
			require.NotNil(t, req.Key)
			seenNS[req.Key.Namespace] = true
			count++
		}
		assert.Equal(t, 2, count)
		assert.True(t, seenNS["ns2"])
		assert.False(t, seenNS["ns1"])
	})

	t.Run("closes all readers on cleanup", func(t *testing.T) {
		mixedReader, err := CreateMixedDataSourceReader(context.Background(), []string{parquetFile1, parquetFile2}, nil, 1024)
		require.NoError(t, err)

		err = mixedReader.Close()
		require.NoError(t, err)
	})

	t.Run("handles empty file list", func(t *testing.T) {
		mixedReader, err := CreateMixedDataSourceReader(context.Background(), []string{}, nil, 1024)
		require.NoError(t, err)

		hasNext := mixedReader.Next()
		require.False(t, hasNext)
	})
}

func TestLabelFilterMatching(t *testing.T) {
	testCases := []struct {
		name     string
		labels   string
		filters  []LabelFilter
		expected bool
	}{
		{
			name:     "empty filters match all",
			labels:   `{"env":"test"}`,
			filters:  nil,
			expected: true,
		},
		{
			name:     "single equal match",
			labels:   `{"env":"test"}`,
			filters:  []LabelFilter{{Key: "env", Value: "test", Op: LabelOpEqual}},
			expected: true,
		},
		{
			name:     "single equal no match",
			labels:   `{"env":"test"}`,
			filters:  []LabelFilter{{Key: "env", Value: "prod", Op: LabelOpEqual}},
			expected: false,
		},
		{
			name:     "not equal match",
			labels:   `{"env":"test"}`,
			filters:  []LabelFilter{{Key: "env", Value: "prod", Op: LabelOpNotEqual}},
			expected: true,
		},
		{
			name:     "regex match",
			labels:   `{"service":"api-gateway"}`,
			filters:  []LabelFilter{{Key: "service", Value: "api.*", Op: LabelOpRegexMatch}},
			expected: true,
		},
		{
			name:     "regex no match",
			labels:   `{"service":"web-app"}`,
			filters:  []LabelFilter{{Key: "service", Value: "api.*", Op: LabelOpRegexMatch}},
			expected: false,
		},
		{
			name:     "multiple filters all match",
			labels:   `{"env":"prod","service":"api"}`,
			filters:  []LabelFilter{{Key: "env", Value: "prod", Op: LabelOpEqual}, {Key: "service", Value: "api", Op: LabelOpEqual}},
			expected: true,
		},
		{
			name:     "multiple filters one fails",
			labels:   `{"env":"prod","service":"web"}`,
			filters:  []LabelFilter{{Key: "env", Value: "prod", Op: LabelOpEqual}, {Key: "service", Value: "api", Op: LabelOpEqual}},
			expected: false,
		},
		{
			name:     "missing label key",
			labels:   `{"env":"test"}`,
			filters:  []LabelFilter{{Key: "missing", Value: "value", Op: LabelOpEqual}},
			expected: false,
		},
		{
			name:     "empty labels with filter",
			labels:   ``,
			filters:  []LabelFilter{{Key: "env", Value: "test", Op: LabelOpEqual}},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reader := &parquetReader{
				labelFilters: tc.filters,
			}

			result := reader.matchesLabelFilters(tc.labels)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParquetWriter(t *testing.T) {
	tmpDir := t.TempDir()
	parquetFile := filepath.Join(tmpDir, "writer_test.parquet")

	t.Run("creates valid parquet file", func(t *testing.T) {
		writer, err := NewWriter(parquetFile, WriteOptions{
			RowGroupSize: 100,
		})
		require.NoError(t, err)

		for i := 0; i < 10; i++ {
			err := writer.Write(context.Background(), &resourcepb.ResourceKey{
				Namespace: "ns",
				Group:    "group",
				Resource: "resource",
				Name:     "name",
			}, []byte(`{"data":"test"}`), `{"index":`+string(rune('0'+i))+`}`)
			require.NoError(t, err)
		}

		result, err := writer.CloseWithResults()
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Nil(t, result.Error)
		require.Equal(t, int64(10), result.Processed)

		_, err = os.Stat(parquetFile)
		require.NoError(t, err)
	})

	t.Run("writes and reads back data", func(t *testing.T) {
		writer, err := NewWriter(parquetFile, WriteOptions{RowGroupSize: 100})
		require.NoError(t, err)

		writeData := []struct {
			key     *resourcepb.ResourceKey
			value   []byte
			labels  string
		}{
			{&resourcepb.ResourceKey{Namespace: "ns1", Group: "g1", Resource: "r1", Name: "n1"}, []byte(`{"data":"1"}`), `{"level":"info"}`},
			{&resourcepb.ResourceKey{Namespace: "ns2", Group: "g2", Resource: "r2", Name: "n2"}, []byte(`{"data":"2"}`), `{"level":"warn"}`},
			{&resourcepb.ResourceKey{Namespace: "ns3", Group: "g3", Resource: "r3", Name: "n3"}, []byte(`{"data":"3"}`), `{"level":"error"}`},
		}

		for _, wd := range writeData {
			err := writer.Write(context.Background(), wd.key, wd.value, wd.labels)
			require.NoError(t, err)
		}

		_, err = writer.CloseWithResults()
		require.NoError(t, err)

		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, nil, 1024)
		require.NoError(t, err)

		count := 0
		for reader.Next() {
			req := reader.Request()
			require.NotNil(t, req)
			require.NotNil(t, req.Key)
			count++
		}
		require.Equal(t, 3, count)
	})
}

func TestBulkRequestIteratorInterface(t *testing.T) {
	t.Run("parquetReader implements BulkRequestIterator", func(t *testing.T) {
		var iterator BulkRequestIterator = &parquetReader{}
		require.NotNil(t, iterator)
	})

	t.Run("MixedParquetReader implements BulkRequestIterator", func(t *testing.T) {
		mixedReader := &MixedParquetReader{}
		var iterator BulkRequestIterator = mixedReader
		require.NotNil(t, iterator)
	})
}

func TestUnstructuredToParquet(t *testing.T) {
	tmpDir := t.TempDir()
	parquetFile := filepath.Join(tmpDir, "unstructured_test.parquet")

	writer, err := NewWriter(parquetFile, WriteOptions{RowGroupSize: 100})
	require.NoError(t, err)

	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"metadata": map[string]any{
				"namespace":       "test-ns",
				"name":            "test-name",
				"resourceVersion": "1234",
				"annotations": map[string]string{
					utils.AnnoKeyFolder: "test-folder",
				},
			},
			"spec": map[string]any{
				"hello": "world",
			},
		},
	}
	obj.SetKind("TestKind")
	obj.SetAPIVersion("test.group/v1")

	data, err := obj.MarshalJSON()
	require.NoError(t, err)

	key := &resourcepb.ResourceKey{
		Namespace: obj.GetNamespace(),
		Resource:  "testkind",
		Group:     "test.group",
		Name:      obj.GetName(),
	}

	err = writer.Write(context.Background(), key, data, `{"source":"test"}`)
	require.NoError(t, err)

	_, err = writer.CloseWithResults()
	require.NoError(t, err)

	reader, err := ReadParquetWithLabels(context.Background(), parquetFile, nil, 1024)
	require.NoError(t, err)

	require.True(t, reader.Next())

	req := reader.Request()
	require.NotNil(t, req)
	require.NotNil(t, req.Key)
	require.Equal(t, "test-ns", req.Key.Namespace)
	require.Equal(t, "test-name", req.Key.Name)
	require.Equal(t, "test.group", req.Key.Group)
	require.Equal(t, "testkind", req.Key.Resource)
	require.NotEmpty(t, req.Value)
}

func BenchmarkParquetReader(b *testing.B) {
	tmpDir := b.TempDir()
	parquetFile := filepath.Join(tmpDir, "bench.parquet")

	writer, err := NewWriter(parquetFile, WriteOptions{RowGroupSize: 10000})
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < 10000; i++ {
		writer.Write(context.Background(), &resourcepb.ResourceKey{
			Namespace: "ns",
			Group:    "group",
			Resource: "resource",
			Name:     "name",
		}, []byte(`{"data":"test","index":`+string(rune('0'+i%10))+`}`), `{"index":"`+string(rune('0'+i%10))+`"}`)
	}

	_, err = writer.CloseWithResults()
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, nil, 1024)
		if err != nil {
			b.Fatal(err)
		}

		count := 0
		for reader.Next() {
			req := reader.Request()
			if req == nil {
				b.Fatal("nil request")
			}
			count++
		}

		if count != 10000 {
			b.Fatalf("expected 10000, got %d", count)
		}
	}
}

func BenchmarkLabelFiltering(b *testing.B) {
	tmpDir := b.TempDir()
	parquetFile := filepath.Join(tmpDir, "bench_filter.parquet")

	writer, err := NewWriter(parquetFile, WriteOptions{RowGroupSize: 10000})
	if err != nil {
		b.Fatal(err)
	}

	for i := 0; i < 10000; i++ {
		env := "prod"
		if i%2 == 0 {
			env = "dev"
		}
		writer.Write(context.Background(), &resourcepb.ResourceKey{
			Namespace: "ns",
			Group:    "group",
			Resource: "resource",
			Name:     "name",
		}, []byte(`{"data":"test"}`), `{"env":"`+env+`"}`)
	}

	_, err = writer.CloseWithResults()
	if err != nil {
		b.Fatal(err)
	}

	filters := []LabelFilter{
		{Key: "env", Value: "prod", Op: LabelOpEqual},
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		reader, err := ReadParquetWithLabels(context.Background(), parquetFile, filters, 1024)
		if err != nil {
			b.Fatal(err)
		}

		count := 0
		for reader.Next() {
			req := reader.Request()
			if req == nil {
				b.Fatal("nil request")
			}
			count++
		}

		if count != 5000 {
			b.Fatalf("expected 5000, got %d", count)
		}
	}
}