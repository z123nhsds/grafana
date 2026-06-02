package mixedtimeseries

import (
	"math"
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

type timeSeriesPoint struct {
	Time  time.Time
	Value float64
}

func alignAndMergeFrames(promFrame *data.Frame, lokiFrame *data.Frame, alignmentMs int64) *data.Frame {
	promPoints := extractTimeSeriesPoints(promFrame)
	lokiPoints := extractTimeSeriesPoints(lokiFrame)

	alignmentDuration := time.Duration(alignmentMs) * time.Millisecond

	allTimes := make(map[int64]bool)
	for _, p := range promPoints {
		alignedTime := alignTime(p.Time, alignmentDuration)
		allTimes[alignedTime.UnixMilli()] = true
	}
	for _, p := range lokiPoints {
		alignedTime := alignTime(p.Time, alignmentDuration)
		allTimes[alignedTime.UnixMilli()] = true
	}

	sortedTimes := make([]int64, 0, len(allTimes))
	for t := range allTimes {
		sortedTimes = append(sortedTimes, t)
	}
	sort.Slice(sortedTimes, func(i, j int) bool {
		return sortedTimes[i] < sortedTimes[j]
	})

	promValues := make(map[int64]float64)
	for _, p := range promPoints {
		alignedTime := alignTime(p.Time, alignmentDuration)
		key := alignedTime.UnixMilli()
		if existing, ok := promValues[key]; ok {
			promValues[key] = (existing + p.Value) / 2
		} else {
			promValues[key] = p.Value
		}
	}

	lokiValues := make(map[int64]float64)
	for _, p := range lokiPoints {
		alignedTime := alignTime(p.Time, alignmentDuration)
		key := alignedTime.UnixMilli()
		if existing, ok := lokiValues[key]; ok {
			lokiValues[key] = (existing + p.Value) / 2
		} else {
			lokiValues[key] = p.Value
		}
	}

	times := make([]time.Time, 0, len(sortedTimes))
	promVals := make([]float64, 0, len(sortedTimes))
	lokiVals := make([]float64, 0, len(sortedTimes))

	for _, t := range sortedTimes {
		tm := time.UnixMilli(t)
		times = append(times, tm)

		if v, ok := promValues[t]; ok {
			promVals = append(promVals, v)
		} else {
			promVals = append(promVals, math.NaN())
		}

		if v, ok := lokiValues[t]; ok {
			lokiVals = append(lokiVals, v)
		} else {
			lokiVals = append(lokiVals, math.NaN())
		}
	}

	mergedFrame := data.NewFrame("merged_timeseries",
		data.NewField("time", nil, times),
		data.NewField("prometheus_value", nil, promVals),
		data.NewField("loki_value", nil, lokiVals),
	)
	mergedFrame.Meta = &data.FrameMeta{
		Custom: map[string]interface{}{
			"alignmentMs": alignmentMs,
			"pointCount":  len(sortedTimes),
		},
	}

	return mergedFrame
}

func alignTime(t time.Time, alignment time.Duration) time.Time {
	ms := t.UnixMilli()
	alignmentMs := alignment.Milliseconds()
	alignedMs := (ms / alignmentMs) * alignmentMs
	return time.UnixMilli(alignedMs)
}

func extractTimeSeriesPoints(frame *data.Frame) []timeSeriesPoint {
	if frame == nil || frame.Rows() == 0 {
		return nil
	}

	timeField := frame.Fields[0]
	if timeField == nil || timeField.Len() == 0 {
		return nil
	}

	var valueField *data.Field
	for i := 1; i < len(frame.Fields); i++ {
		if frame.Fields[i].Type() == data.FieldTypeFloat64 || frame.Fields[i].Type() == data.FieldTypeNullableFloat64 {
			valueField = frame.Fields[i]
			break
		}
	}

	if valueField == nil {
		return nil
	}

	points := make([]timeSeriesPoint, 0, frame.Rows())
	for i := 0; i < frame.Rows(); i++ {
		tm, ok := timeField.ConcreteAt(i)
		if !ok {
			continue
		}

		timeVal, ok := tm.(time.Time)
		if !ok {
			continue
		}

		val, ok := valueField.ConcreteAt(i)
		if !ok {
			continue
		}

		var fval float64
		switch v := val.(type) {
		case float64:
			fval = v
		case *float64:
			if v != nil {
				fval = *v
			} else {
				continue
			}
		default:
			continue
		}

		points = append(points, timeSeriesPoint{
			Time:  timeVal,
			Value: fval,
		})
	}

	return points
}
