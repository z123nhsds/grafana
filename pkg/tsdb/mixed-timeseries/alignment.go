package mixedtimeseries

import (
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func timeAlign(frames data.Frames) data.Frames {
	if len(frames) == 0 {
		return frames
	}

	type timeValue struct {
		t time.Time
		v *float64
	}

	seriesByRefID := make(map[string][]timeValue)
	var allTimestamps []time.Time
	tsSet := make(map[time.Time]struct{})

	for _, frame := range frames {
		if len(frame.Fields) < 2 {
			continue
		}

		timeField := frame.Fields[0]
		if timeField.Type() != data.FieldTypeTime {
			continue
		}

		valueField := frame.Fields[1]
		if valueField.Type() != data.FieldTypeFloat64 && valueField.Type() != data.FieldTypeNullableFloat64 {
			continue
		}

		refID := frame.RefID

		for i := 0; i < frame.Rows(); i++ {
			tVal := timeField.At(i)
			t, ok := tVal.(time.Time)
			if !ok {
				continue
			}

			var v *float64
			if valueField.Type() == data.FieldTypeNullableFloat64 {
				v = valueField.At(i).(*float64)
			} else {
				val := valueField.At(i).(float64)
				v = &val
			}

			if _, exists := tsSet[t]; !exists {
				tsSet[t] = struct{}{}
				allTimestamps = append(allTimestamps, t)
			}

			seriesByRefID[refID] = append(seriesByRefID[refID], timeValue{t: t, v: v})
		}
	}

	if len(allTimestamps) == 0 {
		return frames
	}

	sort.Slice(allTimestamps, func(i, j int) bool {
		return allTimestamps[i].Before(allTimestamps[j])
	})

	result := make(data.Frames, 0, len(frames))

	for _, frame := range frames {
		if len(frame.Fields) < 2 {
			result = append(result, frame)
			continue
		}

		refID := frame.RefID
		values := seriesByRefID[refID]

		valueMap := make(map[time.Time]*float64, len(values))
		for _, tv := range values {
			valueMap[tv.t] = tv.v
		}

		newFrame := data.NewFrame(frame.Name)
		newFrame.RefID = refID

		timeVec := make([]*time.Time, len(allTimestamps))
		for i, ts := range allTimestamps {
			t := ts
			timeVec[i] = &t
		}
		newFrame.Fields = append(newFrame.Fields, data.NewField("Time", nil, timeVec))

		origValueField := frame.Fields[1]
		newValues := make([]*float64, len(allTimestamps))
		for i, ts := range allTimestamps {
			if val, exists := valueMap[ts]; exists {
				newValues[i] = val
			} else {
				newValues[i] = nil
			}
		}

		newField := data.NewField(origValueField.Name, origValueField.Labels, newValues)
		newField.Config = origValueField.Config
		newFrame.Fields = append(newFrame.Fields, newField)

		result = append(result, newFrame)
	}

	return result
}
