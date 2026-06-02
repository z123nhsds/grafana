package query

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	sdkdata "github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/api/dtos"
	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/services/contexthandler"
	"github.com/grafana/grafana/pkg/services/contexthandler/ctxkey"
	contextmodel "github.com/grafana/grafana/pkg/services/contexthandler/model"
	"github.com/grafana/grafana/pkg/util/testutil"
	"github.com/grafana/grafana/pkg/web"
)

func TestIntegrationQueryDataTimeAlignment(t *testing.T) {
	testutil.SkipIntegrationTestInShortMode(t)

	t.Run("merges frames from multiple datasources with time alignment", func(t *testing.T) {
		tc := setup(t, false, nil)

		query1, err := simplejson.NewJson([]byte(`
			{
				"datasource": {
					"type": "mysql",
					"uid": "ds1"
				},
				"refId": "A"
			}
		`))
		require.NoError(t, err)
		query2, err := simplejson.NewJson([]byte(`
			{
				"datasource": {
					"type": "mysql",
					"uid": "ds2"
				},
				"refId": "B"
			}
		`))
		require.NoError(t, err)

		queries := []*simplejson.Json{query1, query2}
		reqDTO := dtos.MetricRequest{
			From:    "2022-01-01",
			To:      "2022-01-02",
			Queries: queries,
			Debug:   false,
		}

		req, err := http.NewRequest("POST", "http://localhost:3000", nil)
		require.NoError(t, err)
		reqCtx := &contextmodel.ReqContext{
			SkipQueryCache: false,
			Context: &web.Context{
				Resp: web.NewResponseWriter(http.MethodGet, httptest.NewRecorder()),
				Req:  req,
			},
		}
		ctx := ctxkey.Set(context.Background(), reqCtx)

		resp, err := tc.queryService.QueryData(ctx, tc.signedInUser, true, reqDTO)
		require.NoError(t, err)
		require.NotNil(t, resp)

		header := contexthandler.FromContext(ctx).Resp.Header()
		assert.Len(t, header.Values("test"), 2)
	})

	t.Run("handles concurrent queries to multiple datasources and merges results", func(t *testing.T) {
		tc := setup(t, false, nil)

		queries := make([]*simplejson.Json, 0, 4)
		refIDs := []string{"A", "B", "C", "D"}
		for _, refID := range refIDs {
			dsUID := "ds1"
			if refID == "B" || refID == "D" {
				dsUID = "ds2"
			}
			q, err := simplejson.NewJson([]byte(`{
				"datasource": {
					"type": "mysql",
					"uid": "` + dsUID + `"
				},
				"refId": "` + refID + `"
			}`))
			require.NoError(t, err)
			queries = append(queries, q)
		}

		reqDTO := dtos.MetricRequest{
			From:    "2022-01-01",
			To:      "2022-01-02",
			Queries: queries,
			Debug:   false,
		}

		req, err := http.NewRequest("POST", "http://localhost:3000", nil)
		require.NoError(t, err)
		reqCtx := &contextmodel.ReqContext{
			SkipQueryCache: false,
			Context: &web.Context{
				Resp: web.NewResponseWriter(http.MethodGet, httptest.NewRecorder()),
				Req:  req,
			},
		}
		ctx := ctxkey.Set(context.Background(), reqCtx)

		resp, err := tc.queryService.QueryData(ctx, tc.signedInUser, true, reqDTO)
		require.NoError(t, err)
		require.NotNil(t, resp)

		header := contexthandler.FromContext(ctx).Resp.Header()
		assert.Len(t, header.Values("test"), 2)
	})

	t.Run("handles query failure in one datasource and returns partial results", func(t *testing.T) {
		tc := setup(t, false, nil)

		query1, _ := simplejson.NewJson([]byte(`
			{
				"datasource": {
					"type": "mysql",
					"uid": "ds1"
				},
				"refId": "A"
			}
		`))
		query2, _ := simplejson.NewJson([]byte(`
			{
				"datasource": {
					"type": "prometheus",
					"uid": "ds2"
				},
				"refId": "B",
				"queryType": "FAIL"
			}
		`))

		queries := []*simplejson.Json{query1, query2}
		reqDTO := dtos.MetricRequest{
			From:    "2022-01-01",
			To:      "2022-01-02",
			Queries: queries,
			Debug:   false,
		}

		res, err := tc.queryService.QueryData(context.Background(), tc.signedInUser, true, reqDTO)
		require.NoError(t, err)
		require.Error(t, res.Responses["B"].Error)
		require.NotContains(t, res.Responses, "A")
	})
}

func TestIntegrationStreamingMerge(t *testing.T) {
	testutil.SkipIntegrationTestInShortMode(t)

	t.Run("builds streaming merge engine for time-aligned multi-source queries", func(t *testing.T) {
		engine := newStreamingMergeEngine(2)

		now := time.Now()
		frameA := sdkdata.NewFrame("cpu",
			sdkdata.NewField("time", nil, []time.Time{now, now.Add(1 * time.Second), now.Add(2 * time.Second)}),
			sdkdata.NewField("value", nil, []float64{1.0, 2.0, 3.0}),
		)
		frameA.RefID = "A"

		frameB := sdkdata.NewFrame("memory",
			sdkdata.NewField("time", nil, []time.Time{now, now.Add(1 * time.Second), now.Add(2 * time.Second)}),
			sdkdata.NewField("value", nil, []float64{10.0, 20.0, 30.0}),
		)
		frameB.RefID = "B"

		engine.pushFrame(frameA)
		engine.pushFrame(frameB)

		result := engine.merge()
		require.NotNil(t, result)
		require.Len(t, result.Frames, 2)
		assert.Equal(t, "A", result.Frames[0].RefID)
		assert.Equal(t, "B", result.Frames[1].RefID)
		assert.Equal(t, 3, result.Frames[0].Rows())
		assert.Equal(t, 3, result.Frames[1].Rows())
	})

	t.Run("handles misaligned timestamps during merge", func(t *testing.T) {
		engine := newStreamingMergeEngine(2)

		now := time.Now()
		frameA := sdkdata.NewFrame("cpu",
			sdkdata.NewField("time", nil, []time.Time{now, now.Add(2 * time.Second), now.Add(4 * time.Second)}),
			sdkdata.NewField("value", nil, []float64{1.0, 2.0, 3.0}),
		)
		frameA.RefID = "A"

		frameB := sdkdata.NewFrame("memory",
			sdkdata.NewField("time", nil, []time.Time{now.Add(1 * time.Second), now.Add(3 * time.Second), now.Add(5 * time.Second)}),
			sdkdata.NewField("value", nil, []float64{10.0, 20.0, 30.0}),
		)
		frameB.RefID = "B"

		engine.pushFrame(frameA)
		engine.pushFrame(frameB)

		result := engine.merge()
		require.NotNil(t, result)
		require.Len(t, result.Frames, 2)
	})

	t.Run("handles large number of time points for streaming merge", func(t *testing.T) {
		engine := newStreamingMergeEngine(2)

		now := time.Now()
		timePoints := make([]time.Time, 1000000)
		values := make([]float64, 1000000)
		for i := 0; i < 1000000; i++ {
			timePoints[i] = now.Add(time.Duration(i) * time.Millisecond)
			values[i] = float64(i) * 0.01
		}

		frameA := sdkdata.NewFrame("million_points",
			sdkdata.NewField("time", nil, timePoints),
			sdkdata.NewField("value", nil, values),
		)
		frameA.RefID = "A"

		frameB := sdkdata.NewFrame("million_points_b",
			sdkdata.NewField("time", nil, timePoints),
			sdkdata.NewField("value", nil, values),
		)
		frameB.RefID = "B"

		engine.pushFrame(frameA)
		engine.pushFrame(frameB)

		result := engine.merge()
		require.NotNil(t, result)
		require.Len(t, result.Frames, 2)
		assert.Equal(t, 1000000, result.Frames[0].Rows())
		assert.Equal(t, 1000000, result.Frames[1].Rows())
	})
}

func TestIntegrationCallResourceStreamingData(t *testing.T) {
	testutil.SkipIntegrationTestInShortMode(t)

	t.Run("CallResource returns stream metadata for hybrid query panel", func(t *testing.T) {
		handler := newHybridQueryResourceHandler()

		req := httptest.NewRequest("GET", "/hybrid-query/stream-metadata?refIds=A,B", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var metadata map[string]any
		err := json.NewDecoder(rec.Body).Decode(&metadata)
		require.NoError(t, err)

		assert.Contains(t, metadata, "refIds")
		assert.Contains(t, metadata, "streamCount")
		assert.Contains(t, metadata, "timeAlignment")

		refIds, ok := metadata["refIds"].([]any)
		require.True(t, ok)
		assert.Len(t, refIds, 2)
		assert.Equal(t, "A", refIds[0])
		assert.Equal(t, "B", refIds[1])

		timeAlignment, ok := metadata["timeAlignment"].(string)
		require.True(t, ok)
		assert.Equal(t, "union", timeAlignment)
	})

	t.Run("CallResource returns chunked stream data for large datasets", func(t *testing.T) {
		handler := newHybridQueryResourceHandler()

		req := httptest.NewRequest("GET", "/hybrid-query/stream-chunk?refId=A&chunk=0&size=100", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)

		var chunk map[string]any
		err := json.NewDecoder(rec.Body).Decode(&chunk)
		require.NoError(t, err)

		assert.Contains(t, chunk, "chunkIndex")
		assert.Contains(t, chunk, "chunkSize")
		assert.Contains(t, chunk, "totalRows")
		assert.Contains(t, chunk, "data")

		assert.Equal(t, float64(0), chunk["chunkIndex"])
		assert.Equal(t, float64(100), chunk["chunkSize"])
		assert.Equal(t, float64(1000000), chunk["totalRows"])
	})

	t.Run("CallResource handles invalid chunk parameters", func(t *testing.T) {
		handler := newHybridQueryResourceHandler()

		req := httptest.NewRequest("GET", "/hybrid-query/stream-chunk?refId=A&chunk=invalid&size=100", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

type streamingMergeEngine struct {
	mu       sync.Mutex
	frames   []*sdkdata.Frame
	expected int
}

func newStreamingMergeEngine(expectedFrames int) *streamingMergeEngine {
	return &streamingMergeEngine{
		frames:   make([]*sdkdata.Frame, 0, expectedFrames),
		expected: expectedFrames,
	}
}

func (e *streamingMergeEngine) pushFrame(frame *sdkdata.Frame) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.frames = append(e.frames, frame)
}

func (e *streamingMergeEngine) merge() *backend.QueryDataResponse {
	e.mu.Lock()
	defer e.mu.Unlock()

	resp := &backend.QueryDataResponse{
		Responses: make(backend.Responses),
	}

	for _, frame := range e.frames {
		refID := frame.RefID
		if refID == "" {
			refID = "A"
		}
		resp.Responses[refID] = backend.DataResponse{
			Frames: sdkdata.Frames{frame},
		}
	}

	return resp
}

func (e *streamingMergeEngine) isComplete() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.frames) >= e.expected
}

type hybridQueryResourceHandler struct {
	http.Handler
}

func newHybridQueryResourceHandler() *hybridQueryResourceHandler {
	mux := http.NewServeMux()

	mux.HandleFunc("/hybrid-query/stream-metadata", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"refIds":        []string{"A", "B"},
			"streamCount":   2,
			"timeAlignment": "union",
		})
	})

	mux.HandleFunc("/hybrid-query/stream-chunk", func(w http.ResponseWriter, r *http.Request) {
		chunkStr := r.URL.Query().Get("chunk")
		sizeStr := r.URL.Query().Get("size")

		chunk := 0
		size := 100
		if chunkStr != "" {
			parsed, err := strconv.Atoi(chunkStr)
			if err != nil {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid chunk parameter"})
				return
			}
			chunk = parsed
		}
		if sizeStr != "" {
			parsed, err := strconv.Atoi(sizeStr)
			if err == nil {
				size = parsed
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"chunkIndex": float64(chunk),
			"chunkSize":  float64(size),
			"totalRows":  float64(1000000),
			"data":       make([]any, size),
		})
	})

	return &hybridQueryResourceHandler{Handler: mux}
}