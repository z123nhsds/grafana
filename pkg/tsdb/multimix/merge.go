package multimix

import (
	"sort"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/data"
)

func mergeTimeSeries(promFrames data.Frames, lokiFrames data.Frames, from, to time.Time, step time.Duration) []mergedTimePoint {
	promPoints := extractTimePoints(promFrames)
	lokiPoints := extractTimePoints(lokiFrames)

	aligned := timeAlign(promPoints, lokiPoints, from, to, step)

	return aligned
}

type timeValue struct {
	Timestamp time.Time
	Value     float64
}

func extractTimePoints(frames data.Frames) []timeValue {
	var points []timeValue

	for _, frame := range frames {
		if frame == nil || len(frame.Fields) < 2 {
			continue
		}

		timeField := frame.Fields[0]
		valueField := frame.Fields[1]

		if timeField == nil || valueField == nil {
			continue
		}

		if timeField.Type() != data.FieldTypeTime {
			continue
		}

		n := timeField.Len()
		if valueField.Len() < n {
			n = valueField.Len()
		}

		for i := 0; i < n; i++ {
			ts, ok := timeField.At(i).(time.Time)
			if !ok {
				continue
			}

			var val float64
			switch v := valueField.At(i).(type) {
			case float64:
				val = v
			case *float64:
				if v != nil {
					val = *v
				} else {
					continue
				}
			default:
				continue
			}

			points = append(points, timeValue{
				Timestamp: ts,
				Value:     val,
			})
		}
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].Timestamp.Before(points[j].Timestamp)
	})

	return points
}

func timeAlign(promPoints, lokiPoints []timeValue, from, to time.Time, step time.Duration) []mergedTimePoint {
	var aligned []mergedTimePoint

	promIdx := 0
	lokiIdx := 0

	for t := from; t.Before(to) || t.Equal(to); t = t.Add(step) {
		point := mergedTimePoint{Timestamp: t}

		for promIdx < len(promPoints) && promPoints[promIdx].Timestamp.Before(t.Add(-step/2)) {
			promIdx++
		}

		if promIdx < len(promPoints) {
			diff := promPoints[promIdx].Timestamp.Sub(t)
			if diff >= -step/2 && diff < step/2 {
				val := promPoints[promIdx].Value
				point.PromValue = &val
			}
		}

		if promIdx > 0 && point.PromValue == nil {
			prev := promPoints[promIdx-1]
			if t.Sub(prev.Timestamp) < step {
				val := prev.Value
				point.PromValue = &val
			}
		}

		for lokiIdx < len(lokiPoints) && lokiPoints[lokiIdx].Timestamp.Before(t.Add(-step/2)) {
			lokiIdx++
		}

		if lokiIdx < len(lokiPoints) {
			diff := lokiPoints[lokiIdx].Timestamp.Sub(t)
			if diff >= -step/2 && diff < step/2 {
				val := lokiPoints[lokiIdx].Value
				point.LokiLogCount = &val
			}
		}

		if lokiIdx > 0 && point.LokiLogCount == nil {
			prev := lokiPoints[lokiIdx-1]
			if t.Sub(prev.Timestamp) < step {
				val := prev.Value
				point.LokiLogCount = &val
			}
		}

		aligned = append(aligned, point)
	}

	return aligned
}

func buildDataFrame(points []mergedTimePoint, refID string) *data.Frame {
	timestamps := make([]time.Time, len(points))
	promValues := make([]*float64, len(points))
	lokiValues := make([]*float64, len(points))

	for i, p := range points {
		timestamps[i] = p.Timestamp
		promValues[i] = p.PromValue
		lokiValues[i] = p.LokiLogCount
	}

	frame := data.NewFrame(refID,
		data.NewField("time", nil, timestamps),
		data.NewField("prometheus_value", nil, promValues).SetConfig(&data.FieldConfig{
			DisplayName: "Prometheus",
			Unit:        "short",
		}),
		data.NewField("loki_log_count", nil, lokiValues).SetConfig(&data.FieldConfig{
			DisplayName: "Loki Logs",
			Unit:        "short",
		}),
	)

	return frame
}