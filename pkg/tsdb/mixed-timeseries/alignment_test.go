package mixedtimeseries

import (
	"math"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeAlign_EmptyFrames(t *testing.T) {
	result := timeAlign(nil)
	assert.Empty(t, result)

	result = timeAlign(data.Frames{})
	assert.Empty(t, result)
}

func TestTimeAlign_LokiEmptyPrometheusHasData(t *testing.T) {
	promTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	promVal := 42.0

	promFrame := data.NewFrame("prometheus")
	promFrame.RefID = "A"
	promFrame.Fields = append(promFrame.Fields, data.NewField("Time", nil, []time.Time{promTime}))
	promFrame.Fields = append(promFrame.Fields, data.NewField("Value", nil, []*float64{&promVal}))

	lokiFrame := data.NewFrame("loki")
	lokiFrame.RefID = "B"
	lokiFrame.Fields = append(lokiFrame.Fields, data.NewField("Time", nil, []time.Time{}))
	lokiFrame.Fields = append(lokiFrame.Fields, data.NewField("Value", nil, []*float64{}))

	frames := data.Frames{promFrame, lokiFrame}
	result := timeAlign(frames)

	require.Len(t, result, 2)

	for _, frame := range result {
		if frame.RefID == "A" {
			require.Equal(t, 1, frame.Fields[1].Len())
			val := frame.Fields[1].At(0).(*float64)
			require.NotNil(t, val)
			assert.Equal(t, 42.0, *val)
		}
	}
}

func TestTimeAlign_PrometheusValuesNotNulledWhenLokiEmpty(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 1, 0, 2, 0, 0, time.UTC)

	v1 := 10.0
	v2 := 20.0
	v3 := 30.0

	promFrame := data.NewFrame("prometheus")
	promFrame.RefID = "A"
	promFrame.Fields = append(promFrame.Fields, data.NewField("Time", nil, []time.Time{t1, t2, t3}))
	promFrame.Fields = append(promFrame.Fields, data.NewField("Value", nil, []*float64{&v1, &v2, &v3}))

	lokiFrame := data.NewFrame("loki")
	lokiFrame.RefID = "B"
	lokiFrame.Fields = append(lokiFrame.Fields, data.NewField("Time", nil, []time.Time{}))
	lokiFrame.Fields = append(lokiFrame.Fields, data.NewField("Value", nil, []*float64{}))

	result := timeAlign(data.Frames{promFrame, lokiFrame})

	require.Len(t, result, 2)

	promResult := result[0]
	require.Equal(t, "A", promResult.RefID)
	require.Equal(t, 3, promResult.Fields[1].Len())

	for i, expected := range []*float64{&v1, &v2, &v3} {
		actual := promResult.Fields[1].At(i).(*float64)
		require.NotNil(t, actual, "Prometheus value at index %d should not be nil", i)
		assert.Equal(t, *expected, *actual)
	}
}

func TestTimeAlign_MixedTimestamps(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC)
	t3 := time.Date(2024, 1, 1, 0, 2, 0, 0, time.UTC)

	v1 := 10.0
	v2 := 20.0

	promFrame := data.NewFrame("prometheus")
	promFrame.RefID = "A"
	promFrame.Fields = append(promFrame.Fields, data.NewField("Time", nil, []time.Time{t1, t3}))
	promFrame.Fields = append(promFrame.Fields, data.NewField("Value", nil, []*float64{&v1, &v2}))

	lokiFrame := data.NewFrame("loki")
	lokiFrame.RefID = "B"
	lokiVal := 100.0
	lokiFrame.Fields = append(lokiFrame.Fields, data.NewField("Time", nil, []time.Time{t2}))
	lokiFrame.Fields = append(lokiFrame.Fields, data.NewField("Value", nil, []*float64{&lokiVal}))

	result := timeAlign(data.Frames{promFrame, lokiFrame})

	require.Len(t, result, 2)

	for _, frame := range result {
		require.Equal(t, 3, frame.Fields[0].Len())

		if frame.RefID == "A" {
			v := frame.Fields[1].At(0).(*float64)
			require.NotNil(t, v)
			assert.Equal(t, 10.0, *v)
			assert.Nil(t, frame.Fields[1].At(1))
			v = frame.Fields[1].At(2).(*float64)
			require.NotNil(t, v)
			assert.Equal(t, 20.0, *v)
		}

		if frame.RefID == "B" {
			assert.Nil(t, frame.Fields[1].At(0))
			v := frame.Fields[1].At(1).(*float64)
			require.NotNil(t, v)
			assert.Equal(t, 100.0, *v)
			assert.Nil(t, frame.Fields[1].At(2))
		}
	}
}

func TestTimeAlign_SortedTimestamps(t *testing.T) {
	t3 := time.Date(2024, 1, 1, 0, 2, 0, 0, time.UTC)
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	t2 := time.Date(2024, 1, 1, 0, 1, 0, 0, time.UTC)

	v1 := 10.0
	v2 := 20.0
	v3 := 30.0

	frame := data.NewFrame("test")
	frame.RefID = "A"
	frame.Fields = append(frame.Fields, data.NewField("Time", nil, []time.Time{t3, t1, t2}))
	frame.Fields = append(frame.Fields, data.NewField("Value", nil, []*float64{&v3, &v1, &v2}))

	result := timeAlign(data.Frames{frame})

	require.Len(t, result, 1)
	require.Equal(t, 3, result[0].Fields[0].Len())

	times := result[0].Fields[0]
	t0 := times.At(0).(time.Time)
	t1r := times.At(1).(time.Time)
	t2r := times.At(2).(time.Time)
	assert.True(t, t0.Before(t1r))
	assert.True(t, t1r.Before(t2r))
}

func TestTimeAlign_DuplicateTimestamps(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	v1 := 10.0
	v2 := 20.0

	frame1 := data.NewFrame("series1")
	frame1.RefID = "A"
	frame1.Fields = append(frame1.Fields, data.NewField("Time", nil, []time.Time{t1}))
	frame1.Fields = append(frame1.Fields, data.NewField("Value", nil, []*float64{&v1}))

	frame2 := data.NewFrame("series2")
	frame2.RefID = "B"
	frame2.Fields = append(frame2.Fields, data.NewField("Time", nil, []time.Time{t1}))
	frame2.Fields = append(frame2.Fields, data.NewField("Value", nil, []*float64{&v2}))

	result := timeAlign(data.Frames{frame1, frame2})

	require.Len(t, result, 2)

	for _, frame := range result {
		require.Equal(t, 1, frame.Fields[0].Len())
	}
}

func TestTimeAlign_FrameWithLessThanTwoFields(t *testing.T) {
	frame := data.NewFrame("incomplete")
	frame.RefID = "A"
	frame.Fields = append(frame.Fields, data.NewField("Time", nil, []time.Time{}))

	result := timeAlign(data.Frames{frame})

	require.Len(t, result, 1)
	assert.Equal(t, frame, result[0])
}

func TestTimeAlign_NaNValues(t *testing.T) {
	t1 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	nan := math.NaN()

	frame := data.NewFrame("test")
	frame.RefID = "A"
	frame.Fields = append(frame.Fields, data.NewField("Time", nil, []time.Time{t1}))
	frame.Fields = append(frame.Fields, data.NewField("Value", nil, []*float64{&nan}))

	result := timeAlign(data.Frames{frame})

	require.Len(t, result, 1)
	require.Equal(t, 1, result[0].Fields[1].Len())
	val := result[0].Fields[1].At(0).(*float64)
	require.NotNil(t, val)
	assert.True(t, math.IsNaN(*val))
}
