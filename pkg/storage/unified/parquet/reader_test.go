package parquet

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/grafana/grafana/pkg/apimachinery/utils"
	"github.com/grafana/grafana/pkg/storage/unified/resource"
	"github.com/grafana/grafana/pkg/storage/unified/resourcepb"
)

func TestParquetWriteThenRead(t *testing.T) {
	t.Run("read-write-couple-rows", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "temp-*.parquet")
		require.NoError(t, err)
		defer func() { _ = os.Remove(file.Name()) }()

		writer, err := NewParquetWriter(file)
		require.NoError(t, err)
		ctx := context.Background()

		require.NoError(t, writer.Write(toKeyAndBytes(ctx, "ggg", "rrr", &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"namespace":       "ns",
					"name":            "aaa",
					"resourceVersion": "1234",
					"annotations": map[string]string{
						utils.AnnoKeyFolder: "xyz",
					},
				},
				"spec": map[string]any{
					"hello": "first",
				},
			},
		})))

		require.NoError(t, writer.Write(toKeyAndBytes(ctx, "ggg", "rrr", &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"namespace":       "ns",
					"name":            "bbb",
					"resourceVersion": "5678",
					"generation":      -999,
				},
				"spec": map[string]any{
					"hello": "second",
				},
			},
		})))

		require.NoError(t, writer.Write(toKeyAndBytes(ctx, "ggg", "rrr", &unstructured.Unstructured{
			Object: map[string]any{
				"metadata": map[string]any{
					"namespace":       "ns",
					"name":            "ccc",
					"resourceVersion": "789",
					"generation":      3,
				},
				"spec": map[string]any{
					"hello": "thirt",
				},
			},
		})))

		res, err := writer.CloseWithResults()
		require.NoError(t, err)
		require.Equal(t, int64(3), res.Processed)

		var keys []string
		reader, err := newResourceReader(file.Name(), 20)
		require.NoError(t, err)
		for reader.Next() {
			req := reader.Request()
			keys = append(keys, resource.SearchID(req.Key))
		}

		require.Equal(t, []string{
			"ns/ggg/rrr/aaa",
			"ns/ggg/rrr/bbb",
			"ns/ggg/rrr/ccc",
		}, keys)
	})

	t.Run("read-write-empty-db", func(t *testing.T) {
		file, err := os.CreateTemp(t.TempDir(), "temp-*.parquet")
		require.NoError(t, err)
		defer func() { _ = os.Remove(file.Name()) }()

		writer, err := NewParquetWriter(file)
		require.NoError(t, err)
		err = writer.Close()
		require.NoError(t, err)

		var keys []string
		reader, err := newResourceReader(file.Name(), 20)
		require.NoError(t, err)
		for reader.Next() {
			req := reader.Request()
			keys = append(keys, resource.SearchID(req.Key))
		}
		require.NoError(t, reader.err)
		require.Empty(t, keys)
	})
}

func TestNewResourceReaderDisablesMemoryMap(t *testing.T) {
	originalOpenParquetFile := openParquetFile
	t.Cleanup(func() {
		openParquetFile = originalOpenParquetFile
	})

	openParquetFile = func(_ string, memoryMap bool) (*file.Reader, error) {
		require.False(t, memoryMap)
		return nil, errors.New("boom")
	}

	reader, err := newResourceReader("ignored.parquet", 20)
	require.Nil(t, reader)
	require.EqualError(t, err, "boom")
}

func toKeyAndBytes(ctx context.Context, group string, res string, obj *unstructured.Unstructured) (context.Context, *resourcepb.ResourceKey, []byte) {
	if obj.GetKind() == "" {
		obj.SetKind(res)
	}
	if obj.GetAPIVersion() == "" {
		obj.SetAPIVersion(group + "/vXyz")
	}
	data, _ := obj.MarshalJSON()
	return ctx, &resourcepb.ResourceKey{
		Namespace: obj.GetNamespace(),
		Resource:  res,
		Group:     group,
		Name:      obj.GetName(),
	}, data
}
