package mixedquery

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/require"
)

// Mock MixedQueryService that implements QueryData and CallResource
type MixedQueryService struct{}

func (s *MixedQueryService) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	resp := backend.NewQueryDataResponse()

	for _, q := range req.Queries {
		// Simulate merging multiple datasources and time alignment
		frame := data.NewFrame("merged_data",
			data.NewField("time", nil, make([]time.Time, 1000000)), // 1M points
			data.NewField("value_ds1", nil, make([]float64, 1000000)),
			data.NewField("value_ds2", nil, make([]float64, 1000000)),
		)

		// Populate with dummy data aligned by time
		baseTime := time.Now().Truncate(time.Hour)
		for i := 0; i < 1000000; i++ {
			frame.Fields[0].Set(i, baseTime.Add(time.Duration(i)*time.Millisecond))
			frame.Fields[1].Set(i, float64(i)*1.5)
			frame.Fields[2].Set(i, float64(i)*0.8)
		}

		resp.Responses[q.RefID] = backend.DataResponse{
			Frames: data.Frames{frame},
		}
	}
	return resp, nil
}

func (s *MixedQueryService) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	// Simulate streaming large chunks of merged data
	for i := 0; i < 10; i++ { // 10 chunks of 100k points
		chunk := map[string]interface{}{
			"chunkIndex": i,
			"points":     100000,
			"timestamp":  time.Now(),
		}
		b, _ := json.Marshal(chunk)
		err := sender.Send(&backend.CallResourceResponse{
			Status: 200,
			Body:   b,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// mockSender implements backend.CallResourceResponseSender
type mockSender struct {
	responses []*backend.CallResourceResponse
}

func (m *mockSender) Send(resp *backend.CallResourceResponse) error {
	m.responses = append(m.responses, resp)
	return nil
}

func TestMixedQuery_QueryData_MillionPoints_Merge(t *testing.T) {
	svc := &MixedQueryService{}

	req := &backend.QueryDataRequest{
		Queries: []backend.DataQuery{
			{RefID: "A"},
		},
	}

	start := time.Now()
	resp, err := svc.QueryData(context.Background(), req)
	duration := time.Since(start)

	require.NoError(t, err)
	require.NotNil(t, resp)
	
	// Verify performance: 1M points merge should take less than 500ms
	require.Less(t, duration.Milliseconds(), int64(500), "QueryData with 1M points took too long")

	resA := resp.Responses["A"]
	require.Len(t, resA.Frames, 1)
	
	frame := resA.Frames[0]
	require.Equal(t, 1000000, frame.Rows())
	require.Equal(t, 3, len(frame.Fields))
	require.Equal(t, "time", frame.Fields[0].Name)
}

func TestMixedQuery_CallResource_Streaming(t *testing.T) {
	svc := &MixedQueryService{}

	req := &backend.CallResourceRequest{
		Path: "stream_merged_data",
	}

	sender := &mockSender{}
	
	start := time.Now()
	err := svc.CallResource(context.Background(), req, sender)
	duration := time.Since(start)

	require.NoError(t, err)
	require.Len(t, sender.responses, 10)
	
	// Verify streaming overhead
	require.Less(t, duration.Milliseconds(), int64(200), "CallResource streaming took too long")

	for i, resp := range sender.responses {
		require.Equal(t, 200, resp.Status)
		var chunk map[string]interface{}
		err := json.Unmarshal(resp.Body, &chunk)
		require.NoError(t, err)
		require.Equal(t, float64(i), chunk["chunkIndex"])
		require.Equal(t, float64(100000), chunk["points"])
	}
}
