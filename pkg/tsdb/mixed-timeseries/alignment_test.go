package mixedtimeseries

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAlignFrames(t *testing.T) {
	step := time.Second

	frameOne := data.NewFrame("frame-1",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1000).UTC(),
			time.UnixMilli(3000).UTC(),
			time.UnixMilli(5000).UTC(),
		}),
		data.NewField("value-1", nil, []float64{1, 3, 5}),
	)

	frameTwo := data.NewFrame("frame-2",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(2000).UTC(),
			time.UnixMilli(3000).UTC(),
			time.UnixMilli(4000).UTC(),
		}),
		data.NewField("value-2", nil, []float64{20, 30, 40}),
	)

	alignedFrames := AlignFrames([]*data.Frame{frameOne, frameTwo}, step)
	require.Len(t, alignedFrames, 2)

	assertAlignedTimeline(t, alignedFrames[0], []int64{1000, 2000, 3000, 4000, 5000})
	assertAlignedValues(t, alignedFrames[0].Fields[1], []float64{1, 0, 3, 0, 5})
	assertAlignedValues(t, alignedFrames[1].Fields[1], []float64{0, 20, 30, 40, 0})
}

func TestAlignFrames_EmptyInput(t *testing.T) {
	assert.Nil(t, AlignFrames(nil, time.Second))
	assert.Nil(t, AlignFrames([]*data.Frame{}, time.Second))
}

func TestAlignFrames_EmptyFrame(t *testing.T) {
	emptyFrame := data.NewFrame("empty",
		data.NewField("time", nil, []time.Time{}),
		data.NewField("value", nil, []float64{}),
	)

	assert.Nil(t, AlignFrames([]*data.Frame{emptyFrame}, time.Second))
}

func TestAlignFrames_IgnoresEmptyFramesWhenTimelineExists(t *testing.T) {
	emptyFrame := data.NewFrame("empty",
		data.NewField("time", nil, []time.Time{}),
		data.NewField("value", nil, []float64{}),
	)
	frame := data.NewFrame("frame",
		data.NewField("time", nil, []time.Time{time.UnixMilli(1000).UTC()}),
		data.NewField("value", nil, []float64{1}),
	)

	alignedFrames := AlignFrames([]*data.Frame{emptyFrame, frame}, time.Second)
	require.Len(t, alignedFrames, 2)
	assertAlignedTimeline(t, alignedFrames[0], []int64{1000})
	assertAlignedValues(t, alignedFrames[0].Fields[1], []float64{0})
	assertAlignedTimeline(t, alignedFrames[1], []int64{1000})
	assertAlignedValues(t, alignedFrames[1].Fields[1], []float64{1})
}

func TestAlignFrames_SkipsFramesWithoutTimeFields(t *testing.T) {
	frame := data.NewFrame("frame-without-time", data.NewField("value", nil, []float64{1}))
	assert.Nil(t, AlignFrames([]*data.Frame{frame}, time.Second))
}

func TestAlignFrames_SinglePoint(t *testing.T) {
	alignedFrames := AlignFrames([]*data.Frame{
		data.NewFrame("frame-1",
			data.NewField("time", nil, []time.Time{time.UnixMilli(1500).UTC()}),
			data.NewField("value-1", nil, []float64{1.5}),
		),
		data.NewFrame("frame-2",
			data.NewField("time", nil, []time.Time{time.UnixMilli(2500).UTC()}),
			data.NewField("value-2", nil, []float64{2.5}),
		),
	}, time.Second)

	require.Len(t, alignedFrames, 2)
	assertAlignedTimeline(t, alignedFrames[0], []int64{2000, 3000})
	assertAlignedValues(t, alignedFrames[0].Fields[1], []float64{1.5, 0})
	assertAlignedValues(t, alignedFrames[1].Fields[1], []float64{0, 2.5})
}

func TestAlignFrames_VariableStep(t *testing.T) {
	frame := data.NewFrame("frame",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1200).UTC(),
			time.UnixMilli(1700).UTC(),
			time.UnixMilli(2200).UTC(),
		}),
		data.NewField("value", nil, []float64{1.2, 1.7, 2.2}),
	)

	alignedAt500ms := AlignFrames([]*data.Frame{frame}, 500*time.Millisecond)
	require.Len(t, alignedAt500ms, 1)
	assertAlignedTimeline(t, alignedAt500ms[0], []int64{1000, 1500, 2000})
	assertAlignedValues(t, alignedAt500ms[0].Fields[1], []float64{1.2, 1.7, 2.2})

	alignedAtOneSecond := AlignFrames([]*data.Frame{frame}, time.Second)
	require.Len(t, alignedAtOneSecond, 1)
	assertAlignedTimeline(t, alignedAtOneSecond[0], []int64{1000, 2000})
	assertAlignedValues(t, alignedAtOneSecond[0].Fields[1], []float64{1.2, 1.7})
}

func TestAlignFrames_PreservesHalfStepMatching(t *testing.T) {
	frame := data.NewFrame("frame",
		data.NewField("time", nil, []time.Time{
			time.UnixMilli(1499).UTC(),
			time.UnixMilli(1500).UTC(),
			time.UnixMilli(2501).UTC(),
		}),
		data.NewField("value", nil, []float64{1.499, 1.5, 2.501}),
	)

	alignedFrames := AlignFrames([]*data.Frame{frame}, time.Second)
	require.Len(t, alignedFrames, 1)
	assertAlignedTimeline(t, alignedFrames[0], []int64{1000, 2000, 3000})
	assertAlignedValues(t, alignedFrames[0].Fields[1], []float64{1.499, 1.5, 2.501})
}

func TestAlignTimestamp(t *testing.T) {
	step := time.Second

	testCases := []struct {
		name     string
		input    time.Time
		expected time.Time
	}{
		{
			name:     "already aligned",
			input:    time.UnixMilli(1000).UTC(),
			expected: time.UnixMilli(1000).UTC(),
		},
		{
			name:     "rounds down within half step",
			input:    time.UnixMilli(1400).UTC(),
			expected: time.UnixMilli(1000).UTC(),
		},
		{
			name:     "rounds up within half step",
			input:    time.UnixMilli(1600).UTC(),
			expected: time.UnixMilli(2000).UTC(),
		},
		{
			name:     "midpoint rounds up",
			input:    time.UnixMilli(1500).UTC(),
			expected: time.UnixMilli(2000).UTC(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, tc.expected.Equal(alignTimestamp(tc.input, step)))
		})
	}
}

func TestAlignFrameRowIndices_UsesFirstPointForSameAlignedTimestamp(t *testing.T) {
	timeField := data.NewField("time", nil, []time.Time{
		time.UnixMilli(1700).UTC(),
		time.UnixMilli(2200).UTC(),
		time.UnixMilli(3200).UTC(),
	})
	targetTimestamps := []time.Time{
		time.UnixMilli(2000).UTC(),
		time.UnixMilli(3000).UTC(),
	}

	assert.Equal(t, []int{0, 2}, alignFrameRowIndices(timeField, targetTimestamps, time.Second))
}

func assertAlignedTimeline(t *testing.T, frame *data.Frame, expected []int64) {
	t.Helper()
	require.NotNil(t, frame)
	require.NotEmpty(t, frame.Fields)

	timeField := frame.Fields[0]
	require.Equal(t, len(expected), timeField.Len())

	for idx, expectedTimestamp := range expected {
		actualTimestamp, ok := timeField.At(idx).(time.Time)
		require.True(t, ok)
		assert.Equal(t, expectedTimestamp, actualTimestamp.UnixMilli())
	}
}

func assertAlignedValues(t *testing.T, field *data.Field, expected []float64) {
	t.Helper()
	require.Equal(t, len(expected), field.Len())

	for idx, expectedValue := range expected {
		actualValue, ok := field.At(idx).(float64)
		require.True(t, ok)
		assert.Equal(t, expectedValue, actualValue)
	}
}
