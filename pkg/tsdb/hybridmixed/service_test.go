package hybridmixed

import (
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/stretchr/testify/require"
)

func TestMergeSeriesAlignsPrometheusAndLokiBuckets(t *testing.T) {
	queryRange := backend.TimeRange{
		From: time.UnixMilli(1000),
		To:   time.UnixMilli(4000),
	}

	promValues := map[int64]float64{
		1000: 1,
		2500: 2,
	}
	lokiValues := map[int64]float64{
		1000: 3,
		3100: 4,
	}

	points := mergeSeries(queryRange, 1000, promValues, lokiValues)
	require.Len(t, points, 4)

	require.Equal(t, int64(1000), points[0].Timestamp)
	require.NotNil(t, points[0].Prometheus)
	require.NotNil(t, points[0].Loki)
	require.NotNil(t, points[0].Merged)
	require.Equal(t, 1.0, *points[0].Prometheus)
	require.Equal(t, 3.0, *points[0].Loki)
	require.Equal(t, 4.0, *points[0].Merged)

	require.Equal(t, int64(2000), points[1].Timestamp)
	require.NotNil(t, points[1].Prometheus)
	require.Nil(t, points[1].Loki)
	require.Equal(t, 2.0, *points[1].Prometheus)
	require.Equal(t, 2.0, *points[1].Merged)

	require.Equal(t, int64(3000), points[2].Timestamp)
	require.Nil(t, points[2].Prometheus)
	require.NotNil(t, points[2].Loki)
	require.Equal(t, 4.0, *points[2].Loki)
	require.Equal(t, 4.0, *points[2].Merged)

	require.Equal(t, int64(4000), points[3].Timestamp)
	require.Nil(t, points[3].Prometheus)
	require.Nil(t, points[3].Loki)
	require.Nil(t, points[3].Merged)
}

func TestBuildFrameKeepsNullableValues(t *testing.T) {
	prometheusValue := 10.0
	points := []mergedPoint{
		{Timestamp: 1000, Prometheus: &prometheusValue, Merged: &prometheusValue},
		{Timestamp: 2000},
	}

	frame := buildFrame("A", points)
	require.Equal(t, "A", frame.RefID)
	require.Len(t, frame.Fields, 4)
	require.Len(t, frame.Fields[0].Values, 2)
	require.Len(t, frame.Fields[1].Values, 2)
	require.Equal(t, time.UnixMilli(1000), frame.Fields[0].At(0).(time.Time))
	require.Equal(t, &prometheusValue, frame.Fields[1].At(0))
	require.Nil(t, frame.Fields[1].At(1))
}
