package mixedtimeseries

import (
	"fmt"
	"sort"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func timeAlign(frames []*data.Frame) ([]*data.Frame, error) {
	if len(frames) == 0 {
		return nil, nil
	}

	type frameWithTimeField struct {
		frame     *data.Frame
		timeField int
	}

	var nonEmpty []frameWithTimeField
	for _, f := range frames {
		if f == nil {
			continue
		}
		if f.Rows() == 0 {
			continue
		}
		timeIdx := findTimeFieldIdx(f)
		if timeIdx < 0 {
			return nil, fmt.Errorf("frame %q has no time field", f.Name)
		}
		nonEmpty = append(nonEmpty, frameWithTimeField{frame: f, timeField: timeIdx})
	}

	if len(nonEmpty) == 0 {
		return nil, nil
	}

	if len(nonEmpty) == 1 {
		return frames, nil
	}

	tsSet := make(map[int64]struct{})
	for _, ft := range nonEmpty {
		timeField := ft.frame.Fields[ft.timeField]
		for i := 0; i < timeField.Len(); i++ {
			t, ok := timeField.ConcreteAt(i)
			if ok {
				tsSet[t.(int64)] = struct{}{}
			}
		}
	}

	if len(tsSet) == 0 {
		return nil, nil
	}

	timestamps := make([]int64, 0, len(tsSet))
	for ts := range tsSet {
		timestamps = append(timestamps, ts)
	}
	sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

	result := make([]*data.Frame, len(frames))
	for i, f := range frames {
		if f == nil || f.Rows() == 0 {
			result[i] = f
			continue
		}
		aligned, err := alignFrameToTimestamps(f, timestamps)
		if err != nil {
			return nil, fmt.Errorf("aligning frame %q: %w", f.Name, err)
		}
		result[i] = aligned
	}

	return result, nil
}

func findTimeFieldIdx(frame *data.Frame) int {
	for i, field := range frame.Fields {
		if field.Type() == data.FieldTypeTime {
			return i
		}
	}
	return -1
}

func alignFrameToTimestamps(frame *data.Frame, timestamps []int64) (*data.Frame, error) {
	timeIdx := findTimeFieldIdx(frame)
	if timeIdx < 0 {
		return nil, fmt.Errorf("no time field found")
	}

	origTimeField := frame.Fields[timeIdx]
	origTimeVals := make([]int64, origTimeField.Len())
	for i := 0; i < origTimeField.Len(); i++ {
		t, ok := origTimeField.ConcreteAt(i)
		if !ok {
			continue
		}
		origTimeVals[i] = t.(int64)
	}

	origIdx := 0
	newFields := make([]*data.Field, len(frame.Fields))

	for fi, field := range frame.Fields {
		if fi != timeIdx {
			origIdx = 0
		}

		newField := data.NewFieldFromFieldType(field.Type(), len(timestamps))
		newField.Name = field.Name
		newField.Labels = field.Labels
		newField.Config = field.Config

		if fi == timeIdx {
			for i, ts := range timestamps {
				newField.Set(i, ts)
			}
		} else {
			for i := 0; i < len(timestamps); i++ {
				if origIdx < len(origTimeVals) && origTimeVals[origIdx] == timestamps[i] {
					val, ok := field.ConcreteAt(origIdx)
					if ok {
						newField.Set(i, val)
					}
					origIdx++
				}
			}
		}

		newFields[fi] = newField
	}

	aligned := data.NewFrame(frame.Name, newFields...)
	aligned.RefID = frame.RefID
	aligned.Meta = frame.Meta

	return aligned, nil
}