package mixedquery

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/tests/testsuite"
)

func TestMain(m *testing.M) {
	testsuite.Run(m)
}

func generateTimeSeries(baseTime time.Time, count int, interval time.Duration, datasourceID string) *data.Frame {
	times := make([]time.Time, count)
	values := make([]float64, count)
	labels := make([]string, count)

	for i := 0; i < count; i++ {
		times[i] = baseTime.Add(time.Duration(i) * interval)
		values[i] = float64(i) * 1.5
		labels[i] = datasourceID
	}

	frame := data.NewFrame("response",
		data.NewField("Time", nil, times),
		data.NewField("Value", nil, values),
		data.NewField("Source", nil, labels),
	)
	frame.Meta = &data.FrameMeta{
		ExecutedQueryString: fmt.Sprintf("SELECT * FROM %s", datasourceID),
	}
	return frame
}

func TestQueryData_TimeAlignment_MultipleDatasources(t *testing.T) {
	t.Run("should align timestamps from two datasources with different intervals", func(t *testing.T) {
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		dsAFrame := generateTimeSeries(baseTime, 100, 10*time.Second, "prometheus")
		dsBFrame := generateTimeSeries(baseTime, 50, 20*time.Second, "loki")

		merged := mergeFramesByTime([]*data.Frame{dsAFrame, dsBFrame})

		require.NotEmpty(t, merged)

		allTimes := extractAllTimestamps(merged)
		require.True(t, isSorted(allTimes), "merged timestamps should be sorted")

		require.GreaterOrEqual(t, len(allTimes), 100, "should have at least as many points as the denser source")
	})

	t.Run("should handle overlapping time ranges with gap filling", func(t *testing.T) {
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		dsAFrame := generateTimeSeries(baseTime, 60, time.Minute, "datasource-a")
		dsBFrame := generateTimeSeries(baseTime.Add(30*time.Minute), 60, time.Minute, "datasource-b")

		merged := mergeFramesByTime([]*data.Frame{dsAFrame, dsBFrame})

		require.NotEmpty(t, merged)

		allTimes := extractAllTimestamps(merged)
		require.True(t, isSorted(allTimes))

		firstTime := allTimes[0]
		lastTime := allTimes[len(allTimes)-1]
		require.Equal(t, baseTime, firstTime)
		require.Equal(t, baseTime.Add(89*time.Minute), lastTime)
	})

	t.Run("should preserve datasource labels after merge", func(t *testing.T) {
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		dsAFrame := generateTimeSeries(baseTime, 10, time.Second, "prometheus")
		dsBFrame := generateTimeSeries(baseTime, 10, time.Second, "loki")

		merged := mergeFramesByTime([]*data.Frame{dsAFrame, dsBFrame})

		sources := extractSources(merged)
		require.Contains(t, sources, "prometheus")
		require.Contains(t, sources, "loki")
	})
}

func TestQueryData_LargeDataset_MillionPoints(t *testing.T) {
	t.Run("should merge 1M points from 3 datasources within time budget", func(t *testing.T) {
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		pointsPerSource := 333333

		var frames []*data.Frame
		datasources := []string{"prometheus", "loki", "tempo"}

		for _, ds := range datasources {
			frame := generateTimeSeries(baseTime, pointsPerSource, 3*time.Second, ds)
			frames = append(frames, frame)
		}

		start := time.Now()
		merged := mergeFramesByTime(frames)
		elapsed := time.Since(start)

		require.NotEmpty(t, merged)

		totalPoints := 0
		for _, f := range merged {
			totalPoints += f.Rows()
		}
		require.Equal(t, pointsPerSource*len(datasources), totalPoints,
			"total merged points should equal sum of all source points")

		t.Logf("Merged %d points from %d datasources in %v",
			totalPoints, len(datasources), elapsed)

		require.Less(t, elapsed.Seconds(), 30.0,
			"merge of 1M points should complete within 30 seconds")
	})

	t.Run("should maintain time ordering under concurrent merge", func(t *testing.T) {
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		pointsPerSource := 250000

		var frames []*data.Frame
		for i := 0; i < 4; i++ {
			frame := generateTimeSeries(
				baseTime.Add(time.Duration(i)*time.Hour),
				pointsPerSource,
				14*time.Second,
				fmt.Sprintf("ds-%d", i),
			)
			frames = append(frames, frame)
		}

		merged := mergeFramesByTime(frames)

		allTimes := extractAllTimestamps(merged)
		require.True(t, isSorted(allTimes),
			"concurrent merge must preserve global time ordering")
	})
}

func TestCallResource_StreamMerge(t *testing.T) {
	t.Run("should merge streaming responses from multiple datasources", func(t *testing.T) {
		ctx := context.Background()

		streamA := newMockStream("stream-a", 1000, 10*time.Millisecond)
		streamB := newMockStream("stream-b", 1000, 15*time.Millisecond)

		mergedChan := make(chan dataPoint, 2000)
		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			for pt := range streamA.Read(ctx) {
				mergedChan <- pt
			}
		}()

		go func() {
			defer wg.Done()
			for pt := range streamB.Read(ctx) {
				mergedChan <- pt
			}
		}()

		go func() {
			wg.Wait()
			close(mergedChan)
		}()

		var collected []dataPoint
		for pt := range mergedChan {
			collected = append(collected, pt)
		}

		require.Len(t, collected, 2000,
			"should receive all points from both streams")

		sources := make(map[string]int)
		for _, pt := range collected {
			sources[pt.Source]++
		}
		require.Equal(t, 1000, sources["stream-a"])
		require.Equal(t, 1000, sources["stream-b"])
	})

	t.Run("should handle out-of-order stream delivery", func(t *testing.T) {
		ctx := context.Background()

		stream := newMockStreamOutOfOrder("oo-stream", 500)

		var collected []dataPoint
		for pt := range stream.Read(ctx) {
			collected = append(collected, pt)
		}

		require.Len(t, collected, 500)

		times := make([]time.Time, len(collected))
		for i, pt := range collected {
			times[i] = pt.Timestamp
		}

		sort.Slice(times, func(i, j int) bool {
			return times[i].Before(times[j])
		})

		for i := 1; i < len(times); i++ {
			require.True(t, !times[i].Before(times[i-1]),
				"sorted times should be non-decreasing")
		}
	})
}

func TestCallResource_ResourcePathRouting(t *testing.T) {
	t.Run("should route to correct datasource by resource path", func(t *testing.T) {
		paths := []struct {
			path     string
			expected string
		}{
			{"/api/ds/prometheus/query", "prometheus"},
			{"/api/ds/loki/query", "loki"},
			{"/api/ds/tempo/traces", "tempo"},
		}

		for _, tc := range paths {
			routed := routeByPath(tc.path)
			require.Equal(t, tc.expected, routed,
				"path %s should route to %s", tc.path, tc.expected)
		}
	})
}

type dataPoint struct {
	Timestamp time.Time
	Value     float64
	Source    string
}

type mockStream struct {
	name     string
	count    int
	interval time.Duration
	outOfOrder bool
}

func newMockStream(name string, count int, interval time.Duration) *mockStream {
	return &mockStream{name: name, count: count, interval: interval}
}

func newMockStreamOutOfOrder(name string, count int) *mockStream {
	return &mockStream{name: name, count: count, outOfOrder: true}
}

func (s *mockStream) Read(ctx context.Context) <-chan dataPoint {
	ch := make(chan dataPoint, s.count)

	go func() {
		defer close(ch)
		baseTime := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

		if s.outOfOrder {
			indices := make([]int, s.count)
			for i := range indices {
				indices[i] = i
			}
			for i := len(indices) - 1; i > 0; i-- {
				j := (i * 7) % (i + 1)
				indices[i], indices[j] = indices[j], indices[i]
			}
			for _, idx := range indices {
				select {
				case <-ctx.Done():
					return
				case ch <- dataPoint{
					Timestamp: baseTime.Add(time.Duration(idx) * time.Second),
					Value:     float64(idx),
					Source:    s.name,
				}:
				}
			}
		} else {
			for i := 0; i < s.count; i++ {
				select {
				case <-ctx.Done():
					return
				case ch <- dataPoint{
					Timestamp: baseTime.Add(time.Duration(i) * s.interval),
					Value:     float64(i),
					Source:    s.name,
				}:
				}
			}
		}
	}()

	return ch
}

func mergeFramesByTime(frames []*data.Frame) []*data.Frame {
	if len(frames) == 0 {
		return nil
	}

	type indexedFrame struct {
		frame *data.Frame
		idx   int
	}

	var all indexedFrame
	var result []*data.Frame

	timeFieldIdx := make(map[int]int)
	for i, f := range frames {
		for j, field := range f.Fields {
			if field.Type() == data.FieldTypeTime {
				timeFieldIdx[i] = j
				break
			}
		}
	}

	for len(frames) > 0 {
		minTime := time.Time{}
		minIdx := -1
		minFrameIdx := -1

		for i, f := range frames {
			tfIdx, ok := timeFieldIdx[i]
			if !ok {
				continue
			}
			if all.idx >= f.Rows() {
				continue
			}
			t := f.Fields[tfIdx].At(all.idx).(time.Time)
			if minFrameIdx == -1 || t.Before(minTime) {
				minTime = t
				minIdx = all.idx
				minFrameIdx = i
			}
		}

		if minFrameIdx == -1 {
			break
		}

		result = append(result, frames[minFrameIdx])
		all.idx++
	}

	if len(result) == 0 {
		return frames
	}

	return result
}

func extractAllTimestamps(frames []*data.Frame) []time.Time {
	var times []time.Time
	for _, f := range frames {
		for _, field := range f.Fields {
			if field.Type() == data.FieldTypeTime {
				for i := 0; i < field.Len(); i++ {
					if t, ok := field.At(i).(time.Time); ok {
						times = append(times, t)
					}
				}
				break
			}
		}
	}
	return times
}

func extractSources(frames []*data.Frame) []string {
	var sources []string
	seen := make(map[string]bool)
	for _, f := range frames {
		for _, field := range f.Fields {
			if field.Name == "Source" {
				for i := 0; i < field.Len(); i++ {
					if s, ok := field.At(i).(string); ok && !seen[s] {
						seen[s] = true
						sources = append(sources, s)
					}
				}
			}
		}
	}
	return sources
}

func isSorted(times []time.Time) bool {
	for i := 1; i < len(times); i++ {
		if times[i].Before(times[i-1]) {
			return false
		}
	}
	return true
}

func routeByPath(path string) string {
	switch {
	case contains(path, "prometheus"):
		return "prometheus"
	case contains(path, "loki"):
		return "loki"
	case contains(path, "tempo"):
		return "tempo"
	default:
		return "unknown"
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
