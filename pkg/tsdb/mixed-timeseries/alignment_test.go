package mixedtimeseries

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/require"
)

func TestTimeAlign_EmptyFrames(t *testing.T) {
	frames := []*data.Frame{}
	result := timeAlign(frames)
	require.Len(t, result, 0)
}

func TestTimeAlign_NilFrames(t *testing.T) {
	frames := []*data.Frame{nil, nil}
	result := timeAlign(frames)
	require.Len(t, result, 2)
	require.Nil(t, result[0])
	require.Nil(t, result[1])
}

func TestTimeAlign_SingleNonEmptyFrame(t *testing.T) {
	frame := data.NewFrame("prom",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1000),
			time.UnixMilli(2000),
			time.UnixMilli(3000),
		}),
		data.NewField("value", nil, []float64{1.0, 2.0, 3.0}),
	)
	frames := []*data.Frame{frame}
	result := timeAlign(frames)
	require.Len(t, result, 1)
	require.Equal(t, 3, result[0].Rows())
}

func TestTimeAlign_EmptyLokiFrameDoesNotNullifyPrometheus(t *testing.T) {
	promFrame := data.NewFrame("prometheus",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1000),
			time.UnixMilli(2000),
			time.UnixMilli(3000),
		}),
		data.NewField("value", nil, []float64{1.0, 2.0, 3.0}),
	)

	lokiFrame := data.NewFrame("loki",
		data.NewField("time", nil, []time.Time{}),
		data.NewField("line", nil, []string{}),
	)

	frames := []*data.Frame{promFrame, lokiFrame}
	result := timeAlign(frames)

	require.Len(t, result, 2)

	promResult := result[0]
	require.Equal(t, 3, promResult.Rows())

	valueField := promResult.Fields[1]
	for i := 0; i < valueField.Len(); i++ {
		v, ok := valueField.At(i).(float64)
		require.True(t, ok, "value at index %d should not be null", i)
		require.Equal(t, float64(i+1), v, "value at index %d should be %f", i, float64(i+1))
	}
}

func TestTimeAlign_NilLokiFrameDoesNotNullifyPrometheus(t *testing.T) {
	promFrame := data.NewFrame("prometheus",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1000),
			time.UnixMilli(2000),
			time.UnixMilli(3000),
		}),
		data.NewField("value", nil, []float64{10.0, 20.0, 30.0}),
	)

	frames := []*data.Frame{promFrame, nil}
	result := timeAlign(frames)

	require.Len(t, result, 2)

	promResult := result[0]
	require.Equal(t, 3, promResult.Rows())

	valueField := promResult.Fields[1]
	for i := 0; i < valueField.Len(); i++ {
		v, ok := valueField.At(i).(float64)
		require.True(t, ok, "value at index %d should not be null", i)
		require.Equal(t, float64((i+1)*10), v)
	}
}

func TestTimeAlign_OverlappingTimePoints(t *testing.T) {
	promFrame := data.NewFrame("prometheus",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1000),
			time.UnixMilli(2000),
			time.UnixMilli(3000),
		}),
		data.NewField("value", nil, []float64{1.0, 2.0, 3.0}),
	)

	lokiMetricFrame := data.NewFrame("loki-metric",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(2000),
			time.UnixMilli(3000),
			time.UnixMilli(4000),
		}),
		data.NewField("value", nil, []float64{20.0, 30.0, 40.0}),
	)

	frames := []*data.Frame{promFrame, lokiMetricFrame}
	result := timeAlign(frames)

	require.Len(t, result, 2)

	promResult := result[0]
	require.Equal(t, 4, promResult.Rows())

	valueField := promResult.Fields[1]
	v0, ok := valueField.At(0).(*float64)
	require.True(t, ok)
	require.NotNil(t, v0)
	require.Equal(t, 1.0, *v0)

	v1, ok := valueField.At(1).(*float64)
	require.True(t, ok)
	require.NotNil(t, v1)
	require.Equal(t, 2.0, *v1)

	v2, ok := valueField.At(2).(*float64)
	require.True(t, ok)
	require.NotNil(t, v2)
	require.Equal(t, 3.0, *v2)

	v3, ok := valueField.At(3).(*float64)
	require.True(t, ok)
	require.Nil(t, v3)
}

func TestTimeAlign_FramesWithSameTimePoints(t *testing.T) {
	promFrame := data.NewFrame("prometheus",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1000),
			time.UnixMilli(2000),
			time.UnixMilli(3000),
		}),
		data.NewField("value", nil, []float64{1.0, 2.0, 3.0}),
	)

	lokiMetricFrame := data.NewFrame("loki-metric",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1000),
			time.UnixMilli(2000),
			time.UnixMilli(3000),
		}),
		data.NewField("value", nil, []float64{10.0, 20.0, 30.0}),
	)

	frames := []*data.Frame{promFrame, lokiMetricFrame}
	result := timeAlign(frames)

	require.Len(t, result, 2)

	promResult := result[0]
	require.Equal(t, 3, promResult.Rows())

	valueField := promResult.Fields[1]
	for i := 0; i < valueField.Len(); i++ {
		v, ok := valueField.At(i).(float64)
		require.True(t, ok, "value at index %d should not be null", i)
		require.Equal(t, float64(i+1), v)
	}
}

func TestTimeAlign_NoTimeField(t *testing.T) {
	frame := data.NewFrame("no-time",
		data.NewField("label", nil, []string{"a", "b"}),
		data.NewField("value", nil, []float64{1.0, 2.0}),
	)

	frames := []*data.Frame{frame}
	result := timeAlign(frames)
	require.Len(t, result, 1)
	require.Equal(t, 2, result[0].Rows())
}
