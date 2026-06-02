package mixedtimeseries

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
)

func TestTimeAlign_EmptyFrames(t *testing.T) {
	result := TimeAlign([]*data.Frame{})
	assert.Empty(t, result)
}

func TestTimeAlign_SingleFrame(t *testing.T) {
	times := []time.Time{time.UnixMilli(1000), time.UnixMilli(2000)}
	values := []float64{1.0, 2.0}
	frame := data.NewFrame("prom",
		data.NewField("time", nil, times),
		data.NewField("value", nil, values),
	)
	result := TimeAlign([]*data.Frame{frame})
	assert.Equal(t, frame, result[0])
}

func TestTimeAlign_SkipsLogFrames(t *testing.T) {
	promTimes := []time.Time{time.UnixMilli(1000), time.UnixMilli(2000)}
	promValues := []float64{1.0, 2.0}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("value", nil, promValues),
	)

	logTimes := []time.Time{time.UnixMilli(1500)}
	logLines := []string{"log line"}
	logFrame := data.NewFrame("loki",
		data.NewField("time", nil, logTimes),
		data.NewField("line", nil, logLines),
	)

	result := TimeAlign([]*data.Frame{promFrame, logFrame})

	promResult := result[0]
	assert.Equal(t, 2, promResult.Rows())
	timeField := promResult.Fields[0]
	valueField := promResult.Fields[1]
	assert.Equal(t, data.FieldTypeTime, timeField.Type())
	for i := 0; i < valueField.Len(); i++ {
		v, ok := valueField.At(i).(float64)
		assert.True(t, ok, "value at index %d should be float64, got nil", i)
		if ok {
			assert.Equal(t, promValues[i], v)
		}
	}
}

func TestTimeAlign_EmptyLokiFrameDoesNotCorruptPrometheus(t *testing.T) {
	promTimes := []time.Time{time.UnixMilli(1000), time.UnixMilli(2000), time.UnixMilli(3000)}
	promValues := []*float64{ptrFloat64(1.0), ptrFloat64(2.0), ptrFloat64(3.0)}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("value", nil, promValues),
	)

	lokiFrame := data.NewFrame("loki-empty",
		data.NewField("time", nil, []time.Time{}),
		data.NewField("line", nil, []string{}),
	)

	result := TimeAlign([]*data.Frame{promFrame, lokiFrame})

	promResult := result[0]
	assert.Equal(t, 3, promResult.Rows())
	valueField := promResult.Fields[1]
	for i := 0; i < 3; i++ {
		v := valueField.At(i)
		assert.NotNil(t, v, "Prometheus value at index %d should not be null", i)
	}
}

func TestTimeAlign_NilFrame(t *testing.T) {
	promTimes := []time.Time{time.UnixMilli(1000)}
	promValues := []float64{1.0}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("value", nil, promValues),
	)

	result := TimeAlign([]*data.Frame{nil, promFrame})
	assert.Nil(t, result[0])
	assert.Equal(t, promFrame, result[1])
}

func TestTimeAlign_AlignsTwoTimeSeriesFrames(t *testing.T) {
	promTimes := []time.Time{time.UnixMilli(1000), time.UnixMilli(3000)}
	promValues := []float64{1.0, 3.0}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("value", nil, promValues),
	)

	lokiMetricTimes := []time.Time{time.UnixMilli(2000), time.UnixMilli(4000)}
	lokiMetricValues := []float64{2.0, 4.0}
	lokiMetricFrame := data.NewFrame("loki-metric",
		data.NewField("time", nil, lokiMetricTimes),
		data.NewField("value", nil, lokiMetricValues),
	)

	result := TimeAlign([]*data.Frame{promFrame, lokiMetricFrame})

	promResult := result[0]
	assert.Equal(t, 4, promResult.Rows())

	timeField := promResult.Fields[0]
	assert.Equal(t, int64(1000), timeField.At(0).(time.Time).UnixMilli())
	assert.Equal(t, int64(2000), timeField.At(1).(time.Time).UnixMilli())
	assert.Equal(t, int64(3000), timeField.At(2).(time.Time).UnixMilli())
	assert.Equal(t, int64(4000), timeField.At(3).(time.Time).UnixMilli())

	valueField := promResult.Fields[1]
	assert.NotNil(t, valueField.At(0))
	assert.Nil(t, valueField.At(1))
	assert.NotNil(t, valueField.At(2))
	assert.Nil(t, valueField.At(3))
}

func TestTimeAlign_NullableFloat64Field(t *testing.T) {
	promTimes := []time.Time{time.UnixMilli(1000), time.UnixMilli(3000)}
	promValues := []*float64{ptrFloat64(1.0), ptrFloat64(3.0)}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("value", nil, promValues),
	)

	lokiMetricTimes := []time.Time{time.UnixMilli(2000)}
	lokiMetricValues := []*float64{ptrFloat64(2.0)}
	lokiMetricFrame := data.NewFrame("loki-metric",
		data.NewField("time", nil, lokiMetricTimes),
		data.NewField("value", nil, lokiMetricValues),
	)

	result := TimeAlign([]*data.Frame{promFrame, lokiMetricFrame})

	promResult := result[0]
	assert.Equal(t, 3, promResult.Rows())
	valueField := promResult.Fields[1]
	assert.Equal(t, data.FieldTypeNullableFloat64, valueField.Type())
	assert.NotNil(t, valueField.At(0))
	assert.Nil(t, valueField.At(1))
	assert.NotNil(t, valueField.At(2))
}

func TestTimeAlign_NullableTimeField(t *testing.T) {
	promTimes := []*time.Time{ptrTime(time.UnixMilli(1000)), ptrTime(time.UnixMilli(3000))}
	promValues := []*float64{ptrFloat64(1.0), ptrFloat64(3.0)}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("value", nil, promValues),
	)

	lokiMetricTimes := []*time.Time{ptrTime(time.UnixMilli(2000))}
	lokiMetricValues := []*float64{ptrFloat64(2.0)}
	lokiMetricFrame := data.NewFrame("loki-metric",
		data.NewField("time", nil, lokiMetricTimes),
		data.NewField("value", nil, lokiMetricValues),
	)

	result := TimeAlign([]*data.Frame{promFrame, lokiMetricFrame})

	promResult := result[0]
	assert.Equal(t, 3, promResult.Rows())
	timeField := promResult.Fields[0]
	assert.Equal(t, data.FieldTypeTime, timeField.Type())
	assert.Equal(t, int64(1000), timeField.At(0).(time.Time).UnixMilli())
	assert.Equal(t, int64(2000), timeField.At(1).(time.Time).UnixMilli())
	assert.Equal(t, int64(3000), timeField.At(2).(time.Time).UnixMilli())
}

func TestTimeAlign_SortedOutput(t *testing.T) {
	promTimes := []time.Time{time.UnixMilli(3000), time.UnixMilli(1000)}
	promValues := []float64{3.0, 1.0}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("value", nil, promValues),
	)

	lokiMetricTimes := []time.Time{time.UnixMilli(2000)}
	lokiMetricValues := []float64{2.0}
	lokiMetricFrame := data.NewFrame("loki-metric",
		data.NewField("time", nil, lokiMetricTimes),
		data.NewField("value", nil, lokiMetricValues),
	)

	result := TimeAlign([]*data.Frame{promFrame, lokiMetricFrame})

	promResult := result[0]
	timeField := promResult.Fields[0]
	for i := 1; i < timeField.Len(); i++ {
		prev := timeField.At(i - 1).(time.Time).UnixMilli()
		curr := timeField.At(i).(time.Time).UnixMilli()
		assert.LessOrEqual(t, prev, curr, "times should be sorted in ascending order")
	}
}

func TestTimeAlign_Int64Field(t *testing.T) {
	promTimes := []time.Time{time.UnixMilli(1000), time.UnixMilli(3000)}
	promValues := []int64{10, 30}
	promFrame := data.NewFrame("prom",
		data.NewField("time", nil, promTimes),
		data.NewField("count", nil, promValues),
	)

	lokiMetricTimes := []time.Time{time.UnixMilli(2000)}
	lokiMetricValues := []int64{20}
	lokiMetricFrame := data.NewFrame("loki-metric",
		data.NewField("time", nil, lokiMetricTimes),
		data.NewField("count", nil, lokiMetricValues),
	)

	result := TimeAlign([]*data.Frame{promFrame, lokiMetricFrame})

	promResult := result[0]
	assert.Equal(t, 3, promResult.Rows())
	valueField := promResult.Fields[1]
	assert.NotNil(t, valueField.At(0))
	assert.Nil(t, valueField.At(1))
	assert.NotNil(t, valueField.At(2))
}

func TestIsTimeSeriesFrame_LogFrame(t *testing.T) {
	logFrame := data.NewFrame("loki",
		data.NewField("time", nil, []time.Time{time.UnixMilli(1000)}),
		data.NewField("line", nil, []string{"log"}),
	)
	assert.False(t, isTimeSeriesFrame(logFrame))
}

func TestIsTimeSeriesFrame_MetricFrame(t *testing.T) {
	metricFrame := data.NewFrame("prom",
		data.NewField("time", nil, []time.Time{time.UnixMilli(1000)}),
		data.NewField("value", nil, []float64{1.0}),
	)
	assert.True(t, isTimeSeriesFrame(metricFrame))
}

func TestIsTimeSeriesFrame_NoTimeField(t *testing.T) {
	frame := data.NewFrame("table",
		data.NewField("name", nil, []string{"a"}),
		data.NewField("value", nil, []float64{1.0}),
	)
	assert.False(t, isTimeSeriesFrame(frame))
}

func ptrFloat64(v float64) *float64 {
	return &v
}

func ptrTime(t time.Time) *time.Time {
	return &t
}
