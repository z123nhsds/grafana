package mixedtimeseries

import (
	"testing"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeAlign_EmptyFrames(t *testing.T) {
	result, err := timeAlign(nil)
	require.NoError(t, err)
	assert.Nil(t, result)

	result, err = timeAlign([]*data.Frame{})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestTimeAlign_SingleFrame(t *testing.T) {
	f := data.NewFrame("test",
		data.NewField("time", nil, []int64{1000, 2000, 3000}),
		data.NewField("value", nil, []float64{1.0, 2.0, 3.0}),
	)

	result, err := timeAlign([]*data.Frame{f})
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, 3, result[0].Rows())
}

func TestTimeAlign_LokiEmptyPreservesPrometheus(t *testing.T) {
	promFrame := data.NewFrame("prometheus",
		data.NewField("time", nil, []int64{1000, 2000, 3000}),
		data.NewField("value", nil, []float64{10.0, 20.0, 30.0}),
	)
	promFrame.RefID = "A"

	lokiFrame := data.NewFrame("loki",
		data.NewField("time", nil, []int64{}),
		data.NewField("value", nil, []float64{}),
	)
	lokiFrame.RefID = "B"

	result, err := timeAlign([]*data.Frame{promFrame, lokiFrame})
	require.NoError(t, err)
	require.Len(t, result, 2)

	promResult := result[0]
	require.NotNil(t, promResult)
	assert.Equal(t, 3, promResult.Rows())

	timeField := promResult.Fields[0]
	assert.Equal(t, int64(1000), timeField.At(0))
	assert.Equal(t, int64(2000), timeField.At(1))
	assert.Equal(t, int64(3000), timeField.At(2))

	valueField := promResult.Fields[1]
	assert.Equal(t, 10.0, valueField.At(0))
	assert.Equal(t, 20.0, valueField.At(1))
	assert.Equal(t, 30.0, valueField.At(2))
}

func TestTimeAlign_AllEmpty(t *testing.T) {
	f1 := data.NewFrame("a",
		data.NewField("time", nil, []int64{}),
		data.NewField("value", nil, []float64{}),
	)
	f2 := data.NewFrame("b",
		data.NewField("time", nil, []int64{}),
		data.NewField("value", nil, []float64{}),
	)

	result, err := timeAlign([]*data.Frame{f1, f2})
	require.NoError(t, err)
	assert.Nil(t, result)
}

func TestTimeAlign_NilFrames(t *testing.T) {
	promFrame := data.NewFrame("prometheus",
		data.NewField("time", nil, []int64{1000, 2000}),
		data.NewField("value", nil, []float64{1.0, 2.0}),
	)

	result, err := timeAlign([]*data.Frame{promFrame, nil})
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.NotNil(t, result[0])
	assert.Nil(t, result[1])
	assert.Equal(t, 2, result[0].Rows())
}

func TestTimeAlign_MultipleNonEmpty(t *testing.T) {
	f1 := data.NewFrame("a",
		data.NewField("time", nil, []int64{1000, 3000}),
		data.NewField("value", nil, []float64{1.0, 3.0}),
	)

	f2 := data.NewFrame("b",
		data.NewField("time", nil, []int64{2000, 4000}),
		data.NewField("value", nil, []float64{2.0, 4.0}),
	)

	result, err := timeAlign([]*data.Frame{f1, f2})
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, 4, result[0].Rows())
	assert.Equal(t, 4, result[1].Rows())
}