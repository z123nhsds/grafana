package mixedtimeseries

import (
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func AlignFrames(frames []*data.Frame, step time.Duration) []*data.Frame {
	if len(frames) == 0 {
		return nil
	}

	targetTimestamps := collectAlignedTimestamps(frames, step)
	if len(targetTimestamps) == 0 {
		return nil
	}

	alignedFrames := make([]*data.Frame, 0, len(frames))
	for _, frame := range frames {
		alignedFrame := alignSingleFrame(frame, targetTimestamps, step)
		if alignedFrame != nil {
			alignedFrames = append(alignedFrames, alignedFrame)
		}
	}

	if len(alignedFrames) == 0 {
		return nil
	}

	return alignedFrames
}

func collectAlignedTimestamps(frames []*data.Frame, step time.Duration) []time.Time {
	timestampSet := make(map[int64]struct{})

	for _, frame := range frames {
		_, timeField := findTimeField(frame)
		if timeField == nil {
			continue
		}

		for rowIdx := 0; rowIdx < timeField.Len(); rowIdx++ {
			ts, ok := fieldTimeValue(timeField.At(rowIdx))
			if !ok {
				continue
			}

			alignedTimestamp := alignTimestamp(ts, step)
			timestampSet[alignedTimestamp.UnixNano()] = struct{}{}
		}
	}

	if len(timestampSet) == 0 {
		return nil
	}

	result := make([]time.Time, 0, len(timestampSet))
	for timestampNano := range timestampSet {
		result = append(result, time.Unix(0, timestampNano).UTC())
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Before(result[j])
	})

	return result
}

func alignTimestamp(ts time.Time, step time.Duration) time.Time {
	stepNano := step.Nanoseconds()
	if stepNano <= 0 {
		return ts.UTC()
	}

	tsNano := ts.UnixNano()
	if tsNano >= 0 {
		return time.Unix(0, ((tsNano+stepNano/2)/stepNano)*stepNano).UTC()
	}

	return time.Unix(0, ((tsNano-stepNano/2)/stepNano)*stepNano).UTC()
}

func alignSingleFrame(frame *data.Frame, targetTimestamps []time.Time, step time.Duration) *data.Frame {
	timeFieldIdx, timeField := findTimeField(frame)
	if timeField == nil || len(targetTimestamps) == 0 {
		return nil
	}

	alignedRowIndices := alignFrameRowIndices(timeField, targetTimestamps, step)

	resultFields := make([]*data.Field, 0, len(frame.Fields))
	alignedTimes := make([]time.Time, len(targetTimestamps))
	copy(alignedTimes, targetTimestamps)
	resultFields = append(resultFields, data.NewField(timeField.Name, timeField.Labels, alignedTimes))

	for fieldIdx, field := range frame.Fields {
		if fieldIdx == timeFieldIdx {
			continue
		}

		alignedField := data.NewFieldFromFieldType(field.Type(), len(targetTimestamps))
		alignedField.Name = field.Name
		if field.Labels != nil {
			alignedField.Labels = field.Labels.Copy()
		}
		if field.Config != nil {
			alignedField.Config = field.Config.Copy()
		}

		for targetIdx, rowIdx := range alignedRowIndices {
			if rowIdx >= 0 {
				alignedField.Set(targetIdx, field.At(rowIdx))
			}
		}

		resultFields = append(resultFields, alignedField)
	}

	result := data.NewFrame(frame.Name, resultFields...)
	result.RefID = frame.RefID
	if frame.Meta != nil {
		result.Meta = frame.Meta.Copy()
	}

	return result
}

func alignFrameRowIndices(timeField *data.Field, targetTimestamps []time.Time, step time.Duration) []int {
	targetTimestampNanos := make([]int64, len(targetTimestamps))
	for idx, targetTimestamp := range targetTimestamps {
		targetTimestampNanos[idx] = targetTimestamp.UnixNano()
	}

	type alignedPoint struct {
		timestampNano int64
		rowIdx        int
	}

	alignedPoints := make([]alignedPoint, 0, timeField.Len())
	for rowIdx := 0; rowIdx < timeField.Len(); rowIdx++ {
		ts, ok := fieldTimeValue(timeField.At(rowIdx))
		if !ok {
			continue
		}

		alignedPoints = append(alignedPoints, alignedPoint{
			timestampNano: alignTimestamp(ts, step).UnixNano(),
			rowIdx:        rowIdx,
		})
	}

	rowIndices := make([]int, len(targetTimestamps))
	for idx := range rowIndices {
		rowIndices[idx] = -1
	}

	pointIdx := 0
	targetIdx := 0
	for pointIdx < len(alignedPoints) && targetIdx < len(targetTimestampNanos) {
		pointTimestampNano := alignedPoints[pointIdx].timestampNano
		targetTimestampNano := targetTimestampNanos[targetIdx]

		switch {
		case pointTimestampNano < targetTimestampNano:
			pointIdx++
		case pointTimestampNano > targetTimestampNano:
			targetIdx++
		default:
			rowIndices[targetIdx] = alignedPoints[pointIdx].rowIdx
			pointIdx++
			targetIdx++
		}
	}

	return rowIndices
}

func findTimeField(frame *data.Frame) (int, *data.Field) {
	if frame == nil {
		return -1, nil
	}

	for fieldIdx, field := range frame.Fields {
		if field.Type() == data.FieldTypeTime || field.Type() == data.FieldTypeNullableTime {
			return fieldIdx, field
		}
	}

	return -1, nil
}

func fieldTimeValue(value any) (time.Time, bool) {
	switch v := value.(type) {
	case time.Time:
		return v, true
	case *time.Time:
		if v == nil {
			return time.Time{}, false
		}
		return *v, true
	default:
		return time.Time{}, false
	}
}
