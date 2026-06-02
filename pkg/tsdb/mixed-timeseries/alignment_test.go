package mixedtimeseries

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlignFrames(t *testing.T) {
	t.Run("对齐多个帧", func(t *testing.T) {
		step := 1000 * time.Millisecond // 1秒步长

		// 帧1：时间戳 [1000, 3000, 5000]
		frame1 := data.NewFrame("frame1",
			data.NewField("time", nil, []time.Time{
				time.UnixMilli(1000).UTC(),
				time.UnixMilli(3000).UTC(),
				time.UnixMilli(5000).UTC(),
			}),
			data.NewField("value1", nil, []float64{1.0, 3.0, 5.0}),
		)

		// 帧2：时间戳 [2000, 3000, 4000]
		frame2 := data.NewFrame("frame2",
			data.NewField("time", nil, []time.Time{
				time.UnixMilli(2000).UTC(),
				time.UnixMilli(3000).UTC(),
				time.UnixMilli(4000).UTC(),
			}),
			data.NewField("value2", nil, []float64{20.0, 30.0, 40.0}),
		)

		alignedFrames := AlignFrames([]*data.Frame{frame1, frame2}, step)
		require.Len(t, alignedFrames, 2)

		// 验证帧1对齐后的数据
		alignedFrame1 := alignedFrames[0]
		require.NotNil(t, alignedFrame1)
		require.Len(t, alignedFrame1.Fields, 2)

		// 检查时间字段
		timeField1 := alignedFrame1.Fields[0]
		require.Equal(t, "time", timeField1.Name)
		require.Equal(t, data.FieldTypeTime, timeField1.Type())
		require.Equal(t, 5, timeField1.Len())
		expectedTimestamps := []int64{1000, 2000, 3000, 4000, 5000}
		for i, expectedTs := range expectedTimestamps {
			ts, ok := timeField1.At(i).(time.Time)
			require.True(t, ok)
			assert.Equal(t, expectedTs, ts.UnixMilli())
		}

		// 检查值字段
		valueField1 := alignedFrame1.Fields[1]
		require.Equal(t, "value1", valueField1.Name)
		require.Equal(t, data.FieldTypeFloat64, valueField1.Type())
		expectedValues1 := []float64{1.0, 0.0, 3.0, 0.0, 5.0}
		for i, expectedVal := range expectedValues1 {
			val, ok := valueField1.At(i).(float64)
			require.True(t, ok)
			assert.Equal(t, expectedVal, val)
		}

		// 验证帧2对齐后的数据
		alignedFrame2 := alignedFrames[1]
		require.NotNil(t, alignedFrame2)
		require.Len(t, alignedFrame2.Fields, 2)

		// 检查值字段
		valueField2 := alignedFrame2.Fields[1]
		require.Equal(t, "value2", valueField2.Name)
		require.Equal(t, data.FieldTypeFloat64, valueField2.Type())
		expectedValues2 := []float64{0.0, 20.0, 30.0, 40.0, 0.0}
		for i, expectedVal := range expectedValues2 {
			val, ok := valueField2.At(i).(float64)
			require.True(t, ok)
			assert.Equal(t, expectedVal, val)
		}
	})

	t.Run("空帧列表", func(t *testing.T) {
		result := AlignFrames([]*data.Frame{}, 1000*time.Millisecond)
		assert.Nil(t, result)
	})

	t.Run("包含空帧", func(t *testing.T) {
		step := 1000 * time.Millisecond
		frame1 := data.NewFrame("frame1",
			data.NewField("time", nil, []time.Time{time.UnixMilli(1000).UTC()}),
			data.NewField("value", nil, []float64{1.0}),
		)
		result := AlignFrames([]*data.Frame{nil, frame1}, step)
		require.Len(t, result, 1)
		assert.Equal(t, "frame1", result[0].Name)
	})

	t.Run("没有时间字段的帧", func(t *testing.T) {
		step := 1000 * time.Millisecond
		frame1 := data.NewFrame("frame1",
			data.NewField("value", nil, []float64{1.0}),
		)
		result := AlignFrames([]*data.Frame{frame1}, step)
		assert.Len(t, result, 0)
	})
}

func TestAlignFrames_SinglePoint(t *testing.T) {
	t.Run("单点帧", func(t *testing.T) {
		step := 1000 * time.Millisecond

		frame1 := data.NewFrame("frame1",
			data.NewField("time", nil, []time.Time{time.UnixMilli(1500).UTC()}),
			data.NewField("value1", nil, []float64{1.5}),
		)

		frame2 := data.NewFrame("frame2",
			data.NewField("time", nil, []time.Time{time.UnixMilli(2500).UTC()}),
			data.NewField("value2", nil, []float64{2.5}),
		)

		alignedFrames := AlignFrames([]*data.Frame{frame1, frame2}, step)
		require.Len(t, alignedFrames, 2)

		// 验证对齐后的时间戳
		timeField1 := alignedFrames[0].Fields[0]
		require.Equal(t, 2, timeField1.Len())
		ts1, _ := timeField1.At(0).(time.Time)
		assert.Equal(t, int64(2000), ts1.UnixMilli()) // 1500 会对齐到 2000
		ts2, _ := timeField1.At(1).(time.Time)
		assert.Equal(t, int64(3000), ts2.UnixMilli()) // 2500 会对齐到 3000
	})
}

func TestAlignFrames_VariableStep(t *testing.T) {
	t.Run("不同步长测试", func(t *testing.T) {
		step500 := 500 * time.Millisecond
		step1000 := 1000 * time.Millisecond

		frame := data.NewFrame("frame",
			data.NewField("time", nil, []time.Time{
				time.UnixMilli(1200).UTC(),
				time.UnixMilli(1700).UTC(),
				time.UnixMilli(2200).UTC(),
			}),
			data.NewField("value", nil, []float64{1.2, 1.7, 2.2}),
		)

		// 500ms 步长对齐
		aligned500 := AlignFrames([]*data.Frame{frame}, step500)
		require.Len(t, aligned500, 1)
		timeField500 := aligned500[0].Fields[0]
		require.Equal(t, 3, timeField500.Len())
		ts500_1, _ := timeField500.At(0).(time.Time)
		ts500_2, _ := timeField500.At(1).(time.Time)
		ts500_3, _ := timeField500.At(2).(time.Time)
		assert.Equal(t, int64(1000), ts500_1.UnixMilli())
		assert.Equal(t, int64(1500), ts500_2.UnixMilli())
		assert.Equal(t, int64(2000), ts500_3.UnixMilli())

		// 1000ms 步长对齐
		aligned1000 := AlignFrames([]*data.Frame{frame}, step1000)
		require.Len(t, aligned1000, 1)
		timeField1000 := aligned1000[0].Fields[0]
		require.Equal(t, 2, timeField1000.Len())
		ts1000_1, _ := timeField1000.At(0).(time.Time)
		ts1000_2, _ := timeField1000.At(1).(time.Time)
		assert.Equal(t, int64(1000), ts1000_1.UnixMilli())
		assert.Equal(t, int64(2000), ts1000_2.UnixMilli())
	})
}

func TestAlignTimestamp(t *testing.T) {
	step := 1000 * time.Millisecond

	testCases := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "精确对齐",
			input:    time.UnixMilli(1000).UTC(),
			expected: time.UnixMilli(1000).UTC(),
		},
		{
			name:     "向下对齐",
			input:    time.UnixMilli(1400).UTC(),
			expected: time.UnixMilli(1000).UTC(),
		},
		{
			name:     "向上对齐",
			input:    time.UnixMilli(1600).UTC(),
			expected: time.UnixMilli(2000).UTC(),
		},
		{
			name:     "中点向上对齐",
			input:    time.UnixMilli(1500).UTC(),
			expected: time.UnixMilli(2000).UTC(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := alignTimestamp(tc.input, step)
			assert.Equal(t, tc.expected.UnixMilli(), result.UnixMilli())
		})
	}
}

