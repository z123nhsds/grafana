package mixedtimeseries

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ts(sec int64) time.Time {
	return time.Unix(sec, 0)
}

func makeFrame(name string, times []time.Time, values []float64) *data.Frame {
	return data.NewFrame(name,
		data.NewField("time", nil, times),
		data.NewField("value", nil, values),
	)
}

func TestAlignTimeFields_ExactMatch(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10), ts(20), ts(30)})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10), ts(20), ts(30)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Len(t, pairs, 4)
	for i, p := range pairs {
		assert.Equal(t, i, p.refIdx)
		assert.Equal(t, i, p.tgtIdx)
	}
}

func TestAlignTimeFields_WithinHalfStep(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10), ts(20)})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(1), ts(11), ts(21)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Len(t, pairs, 3)
}

func TestAlignTimeFields_OutsideHalfStep(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10), ts(20)})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(6), ts(16), ts(26)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Empty(t, pairs)
}

func TestAlignTimeFields_PartialOverlap(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10), ts(20), ts(30)})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(10), ts(20), ts(30)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Len(t, pairs, 3)
	assert.Equal(t, 1, pairs[0].refIdx)
	assert.Equal(t, 0, pairs[0].tgtIdx)
}

func TestAlignTimeFields_EmptyFields(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Empty(t, pairs)

	pairs = alignTimeFields(tgtTimes, refTimes, 10*time.Second)
	assert.Empty(t, pairs)
}

func TestAlignTimeFields_SinglePoint(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(10)})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(12)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Len(t, pairs, 1)

	tgtTimes2 := data.NewField("time", nil, []time.Time{ts(16)})
	pairs = alignTimeFields(refTimes, tgtTimes2, 10*time.Second)
	assert.Empty(t, pairs)
}

func TestAlignTimeFields_StepChange(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10), ts(20), ts(30), ts(40)})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(0), ts(20), ts(40)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Len(t, pairs, 3)
	assert.Equal(t, 0, pairs[0].refIdx)
	assert.Equal(t, 0, pairs[0].tgtIdx)
	assert.Equal(t, 2, pairs[1].refIdx)
	assert.Equal(t, 1, pairs[1].tgtIdx)
	assert.Equal(t, 4, pairs[2].refIdx)
	assert.Equal(t, 2, pairs[2].tgtIdx)
}

func TestAlignToReference_BasicAlignment(t *testing.T) {
	ref := makeFrame("ref", []time.Time{ts(0), ts(10), ts(20)}, []float64{1, 2, 3})
	tgt := makeFrame("tgt", []time.Time{ts(1), ts(11), ts(21)}, []float64{10, 20, 30})

	results, err := AlignToReference(ref, []*data.Frame{tgt}, 10*time.Second)
	require.NoError(t, err)
	require.Len(t, results, 1)

	aligned := results[0]
	require.GreaterOrEqual(t, len(aligned.Fields), 2)

	timeField := aligned.Fields[0]
	assert.Equal(t, data.FieldTypeTime, timeField.Type())
	assert.Equal(t, 3, timeField.Len())
	assert.Equal(t, ts(0), timeField.At(0).(time.Time))
	assert.Equal(t, ts(10), timeField.At(1).(time.Time))
	assert.Equal(t, ts(20), timeField.At(2).(time.Time))

	valueField := aligned.Fields[1]
	assert.Equal(t, 10.0, valueField.At(0).(float64))
	assert.Equal(t, 20.0, valueField.At(1).(float64))
	assert.Equal(t, 30.0, valueField.At(2).(float64))
}

func TestAlignToReference_EmptyFrame(t *testing.T) {
	ref := makeFrame("ref", []time.Time{}, []float64{})
	tgt := makeFrame("tgt", []time.Time{ts(0)}, []float64{1})

	results, err := AlignToReference(ref, []*data.Frame{tgt}, 10*time.Second)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 0, results[0].Fields[0].Len())
}

func TestAlignToReference_SinglePointFrame(t *testing.T) {
	ref := makeFrame("ref", []time.Time{ts(10)}, []float64{1})
	tgt := makeFrame("tgt", []time.Time{ts(12)}, []float64{5})

	results, err := AlignToReference(ref, []*data.Frame{tgt}, 10*time.Second)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, 1, results[0].Fields[0].Len())
}

func TestAlignToReference_Errors(t *testing.T) {
	t.Run("nil reference", func(t *testing.T) {
		_, err := AlignToReference(nil, []*data.Frame{{}}, 10*time.Second)
		assert.ErrorIs(t, err, ErrNilFrame)
	})

	t.Run("nil target", func(t *testing.T) {
		ref := makeFrame("ref", []time.Time{ts(0)}, []float64{1})
		_, err := AlignToReference(ref, []*data.Frame{nil}, 10*time.Second)
		assert.ErrorIs(t, err, ErrNilFrame)
	})

	t.Run("zero step", func(t *testing.T) {
		ref := makeFrame("ref", []time.Time{ts(0)}, []float64{1})
		_, err := AlignToReference(ref, []*data.Frame{ref}, 0)
		assert.ErrorIs(t, err, ErrStepZero)
	})

	t.Run("no targets", func(t *testing.T) {
		ref := makeFrame("ref", []time.Time{ts(0)}, []float64{1})
		_, err := AlignToReference(ref, nil, 10*time.Second)
		assert.ErrorIs(t, err, ErrFrameCount)
	})

	t.Run("no time field in target", func(t *testing.T) {
		ref := makeFrame("ref", []time.Time{ts(0)}, []float64{1})
		tgt := data.NewFrame("tgt", data.NewField("value", nil, []float64{1}))
		_, err := AlignToReference(ref, []*data.Frame{tgt}, 10*time.Second)
		assert.ErrorIs(t, err, ErrNoTimeField)
	})
}

func TestMergeFrames_BasicMerge(t *testing.T) {
	f1 := makeFrame("f1", []time.Time{ts(0), ts(10), ts(20)}, []float64{1, 2, 3})
	f2 := makeFrame("f2", []time.Time{ts(1), ts(11), ts(21)}, []float64{10, 20, 30})

	merged, err := MergeFrames([]*data.Frame{f1, f2}, 10*time.Second)
	require.NoError(t, err)

	timeField := merged.Fields[0]
	assert.Equal(t, 3, timeField.Len())

	valueFields := 0
	for _, f := range merged.Fields {
		if f.Type() == data.FieldTypeFloat64 {
			valueFields++
		}
	}
	assert.Equal(t, 2, valueFields)
}

func TestMergeFrames_SingleFrame(t *testing.T) {
	f1 := makeFrame("f1", []time.Time{ts(0), ts(10)}, []float64{1, 2})

	merged, err := MergeFrames([]*data.Frame{f1}, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, f1, merged)
}

func TestMergeFrames_EmptyFrames(t *testing.T) {
	f1 := makeFrame("f1", []time.Time{}, []float64{})
	f2 := makeFrame("f2", []time.Time{}, []float64{})

	merged, err := MergeFrames([]*data.Frame{f1, f2}, 10*time.Second)
	require.NoError(t, err)
	assert.Equal(t, 0, merged.Fields[0].Len())
}

func TestMergeFrames_Errors(t *testing.T) {
	t.Run("empty input", func(t *testing.T) {
		_, err := MergeFrames(nil, 10*time.Second)
		assert.ErrorIs(t, err, ErrFrameCount)
	})

	t.Run("nil frame", func(t *testing.T) {
		f1 := makeFrame("f1", []time.Time{ts(0)}, []float64{1})
		_, err := MergeFrames([]*data.Frame{f1, nil}, 10*time.Second)
		assert.ErrorIs(t, err, ErrNilFrame)
	})

	t.Run("zero step", func(t *testing.T) {
		f1 := makeFrame("f1", []time.Time{ts(0)}, []float64{1})
		_, err := MergeFrames([]*data.Frame{f1, f1}, 0)
		assert.ErrorIs(t, err, ErrStepZero)
	})
}

func TestMergeFrames_PartialOverlap(t *testing.T) {
	f1 := makeFrame("f1", []time.Time{ts(0), ts(10), ts(20), ts(30)}, []float64{1, 2, 3, 4})
	f2 := makeFrame("f2", []time.Time{ts(10), ts(20), ts(30)}, []float64{10, 20, 30})

	merged, err := MergeFrames([]*data.Frame{f1, f2}, 10*time.Second)
	require.NoError(t, err)

	timeField := merged.Fields[0]
	assert.Equal(t, 4, timeField.Len())

	f1ValueField := merged.Fields[1]
	assert.Equal(t, 1.0, f1ValueField.At(0).(float64))
	assert.Equal(t, 2.0, f1ValueField.At(1).(float64))

	f2ValueField := merged.Fields[2]
	assert.Nil(t, f2ValueField.At(0))
	assert.Equal(t, 10.0, f2ValueField.At(1).(float64))
}

func TestAlignTimeFields_BoundaryHalfStep(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(10)})
	tgtAtPlus5 := data.NewField("time", nil, []time.Time{ts(15)})
	tgtAtMinus5 := data.NewField("time", nil, []time.Time{ts(5)})

	pairs := alignTimeFields(refTimes, tgtAtPlus5, 10*time.Second)
	assert.Len(t, pairs, 1, "exactly +step/2 should match")

	pairs = alignTimeFields(refTimes, tgtAtMinus5, 10*time.Second)
	assert.Len(t, pairs, 1, "exactly -step/2 should match")

	tgtAtPlus6 := data.NewField("time", nil, []time.Time{ts(16)})
	pairs = alignTimeFields(refTimes, tgtAtPlus6, 10*time.Second)
	assert.Empty(t, pairs, "beyond +step/2 should not match")
}

func TestAlignTimeFields_DifferentLengths(t *testing.T) {
	refTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10), ts(20), ts(30), ts(40)})
	tgtTimes := data.NewField("time", nil, []time.Time{ts(0), ts(10)})

	pairs := alignTimeFields(refTimes, tgtTimes, 10*time.Second)
	assert.Len(t, pairs, 2)
	assert.Equal(t, 0, pairs[0].refIdx)
	assert.Equal(t, 0, pairs[0].tgtIdx)
	assert.Equal(t, 1, pairs[1].refIdx)
	assert.Equal(t, 1, pairs[1].tgtIdx)
}

func TestExtractTimeField(t *testing.T) {
	t.Run("nil frame", func(t *testing.T) {
		_, err := extractTimeField(nil)
		assert.ErrorIs(t, err, ErrNilFrame)
	})

	t.Run("no time field", func(t *testing.T) {
		frame := data.NewFrame("test", data.NewField("value", nil, []float64{1}))
		_, err := extractTimeField(frame)
		assert.ErrorIs(t, err, ErrNoTimeField)
	})

	t.Run("has time field", func(t *testing.T) {
		frame := makeFrame("test", []time.Time{ts(0)}, []float64{1})
		f, err := extractTimeField(frame)
		assert.NoError(t, err)
		assert.Equal(t, data.FieldTypeTime, f.Type())
	})
}

func BenchmarkAlignTimeFields(b *testing.B) {
	size := 10000
	refTimes := make([]time.Time, size)
	tgtTimes := make([]time.Time, size)
	for i := 0; i < size; i++ {
		refTimes[i] = ts(int64(i * 10))
		tgtTimes[i] = ts(int64(i*10 + 1))
	}

	refField := data.NewField("time", nil, refTimes)
	tgtField := data.NewField("time", nil, tgtTimes)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		alignTimeFields(refField, tgtField, 10*time.Second)
	}
}
