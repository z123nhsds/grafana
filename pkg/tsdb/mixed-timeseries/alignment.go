package mixedtimeseries

import (
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func TimeAlign(frames []*data.Frame) []*data.Frame {
	if len(frames) == 0 {
		return frames
	}

	nonEmptyFrames := make([]*data.Frame, 0, len(frames))
	for _, f := range frames {
		if f != nil && f.Rows() > 0 && isTimeSeriesFrame(f) {
			nonEmptyFrames = append(nonEmptyFrames, f)
		}
	}

	if len(nonEmptyFrames) == 0 {
		return frames
	}

	if len(nonEmptyFrames) == 1 {
		return frames
	}

	timeSet := make(map[int64]struct{})
	for _, f := range nonEmptyFrames {
		timeField := findTimeField(f)
		if timeField == nil {
			continue
		}
		for i := 0; i < timeField.Len(); i++ {
			if ms, ok := extractTimeMilli(timeField, i); ok {
				timeSet[ms] = struct{}{}
			}
		}
	}

	if len(timeSet) == 0 {
		return frames
	}

	allTimes := make([]int64, 0, len(timeSet))
	for t := range timeSet {
		allTimes = append(allTimes, t)
	}
	sort.Slice(allTimes, func(i, j int) bool { return allTimes[i] < allTimes[j] })

	result := make([]*data.Frame, len(frames))
	for idx, f := range frames {
		if f == nil {
			continue
		}

		timeField := findTimeField(f)
		if timeField == nil || timeField.Len() == 0 {
			result[idx] = f
			continue
		}

		existingTimes := make(map[int64]int)
		for i := 0; i < timeField.Len(); i++ {
			if ms, ok := extractTimeMilli(timeField, i); ok {
				existingTimes[ms] = i
			}
		}

		if len(existingTimes) == 0 {
			result[idx] = f
			continue
		}

		needsAlignment := false
		for _, t := range allTimes {
			if _, exists := existingTimes[t]; !exists {
				needsAlignment = true
				break
			}
		}

		if !needsAlignment {
			result[idx] = f
			continue
		}

		result[idx] = alignFrameToTimes(f, allTimes, existingTimes)
	}

	return result
}

func alignFrameToTimes(frame *data.Frame, allTimes []int64, existingTimes map[int64]int) *data.Frame {
	newFields := make(data.Fields, len(frame.Fields))

	for fieldIdx, field := range frame.Fields {
		ft := field.Type()

		if ft == data.FieldTypeTime || ft == data.FieldTypeNullableTime {
			times := make([]time.Time, len(allTimes))
			for i, ms := range allTimes {
				times[i] = time.UnixMilli(ms)
			}
			newFields[fieldIdx] = data.NewField(field.Name, field.Labels, times)
			if field.Config != nil {
				newFields[fieldIdx].Config = field.Config
			}
			continue
		}

		switch ft {
		case data.FieldTypeFloat64, data.FieldTypeNullableFloat64:
			values := make([]*float64, len(allTimes))
			for i, ms := range allTimes {
				if rowIdx, exists := existingTimes[ms]; exists && rowIdx < field.Len() {
					if v, ok := field.At(rowIdx).(*float64); ok {
						values[i] = v
					} else if v, ok := field.At(rowIdx).(float64); ok {
						values[i] = &v
					}
				}
			}
			newFields[fieldIdx] = data.NewField(field.Name, field.Labels, values)
		case data.FieldTypeInt64, data.FieldTypeNullableInt64:
			values := make([]*int64, len(allTimes))
			for i, ms := range allTimes {
				if rowIdx, exists := existingTimes[ms]; exists && rowIdx < field.Len() {
					if v, ok := field.At(rowIdx).(*int64); ok {
						values[i] = v
					} else if v, ok := field.At(rowIdx).(int64); ok {
						values[i] = &v
					}
				}
			}
			newFields[fieldIdx] = data.NewField(field.Name, field.Labels, values)
		case data.FieldTypeString, data.FieldTypeNullableString:
			values := make([]*string, len(allTimes))
			for i, ms := range allTimes {
				if rowIdx, exists := existingTimes[ms]; exists && rowIdx < field.Len() {
					if v, ok := field.At(rowIdx).(*string); ok {
						values[i] = v
					} else if v, ok := field.At(rowIdx).(string); ok {
						values[i] = &v
					}
				}
			}
			newFields[fieldIdx] = data.NewField(field.Name, field.Labels, values)
		default:
			newFields[fieldIdx] = field
		}

		if field.Config != nil && newFields[fieldIdx].Config == nil {
			newFields[fieldIdx].Config = field.Config
		}
	}

	return data.NewFrame(frame.Name, newFields...).SetMeta(frame.Meta)
}

func findTimeField(frame *data.Frame) *data.Field {
	for _, field := range frame.Fields {
		if field.Type() == data.FieldTypeTime || field.Type() == data.FieldTypeNullableTime {
			return field
		}
	}
	return nil
}

func extractTimeMilli(field *data.Field, idx int) (int64, bool) {
	v := field.At(idx)
	switch tv := v.(type) {
	case time.Time:
		return tv.UnixMilli(), true
	case *time.Time:
		if tv != nil {
			return tv.UnixMilli(), true
		}
	}
	return 0, false
}

func isTimeSeriesFrame(frame *data.Frame) bool {
	timeField := findTimeField(frame)
	if timeField == nil {
		return false
	}
	for _, field := range frame.Fields {
		ft := field.Type()
		if ft == data.FieldTypeFloat64 || ft == data.FieldTypeNullableFloat64 ||
			ft == data.FieldTypeInt64 || ft == data.FieldTypeNullableInt64 {
			return true
		}
	}
	return false
}
