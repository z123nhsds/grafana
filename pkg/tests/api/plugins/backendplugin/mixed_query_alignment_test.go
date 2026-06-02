package backendplugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/stretchr/testify/require"

	"github.com/grafana/grafana/pkg/components/simplejson"
	"github.com/grafana/grafana/pkg/plugins"
	"github.com/grafana/grafana/pkg/services/datasources"
	"github.com/grafana/grafana/pkg/tests/testinfra"
	"github.com/grafana/grafana/pkg/util/testutil"
)

func TestIntegrationMixedDatasourceAlignmentContract(t *testing.T) {
	testutil.SkipIntegrationTestInShortMode(t)

	dir, path := testinfra.CreateGrafDir(t, testinfra.GrafanaOpts{DisableAnonymous: true})
	grafanaListeningAddr, testEnv := testinfra.StartGrafanaEnv(t, dir, path)
	testEnv.Cfg.LoginCookieName = loginCookieName

	pluginID := "test-mixed-alignment-plugin"
	leftUID := "mixed-left"
	rightUID := "mixed-right"

	alignmentRows := []alignedRow{
		{Timestamp: 1000, Left: float64Pointer(1)},
		{Timestamp: 2000, Right: float64Pointer(20)},
		{Timestamp: 3000, Left: float64Pointer(3), Right: float64Pointer(30)},
		{Timestamp: 4000, Right: float64Pointer(40)},
	}

	ctx := context.Background()
	tsCtx := &testScenarioContext{grafanaListeningAddr: grafanaListeningAddr, testEnv: testEnv}
	plugin, backendPlugin := createTestPlugin(pluginID, tsCtx)
	plugin.Class = plugins.ClassCore

	backendPlugin.QueryDataHandler = backend.QueryDataHandlerFunc(func(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
		response := backend.NewQueryDataResponse()
		datasourceUID := req.PluginContext.DataSourceInstanceSettings.UID

		for _, query := range req.Queries {
			response.Responses[query.RefID] = backend.DataResponse{
				Frames: data.Frames{buildAlignedFrame(query.RefID, datasourceUID, alignmentRows)},
			}
		}

		return response, nil
	})

	backendPlugin.CallResourceHandler = backend.CallResourceHandlerFunc(func(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
		payload := struct {
			Rows []alignedRow `json:"rows"`
		}{Rows: alignmentRows}

		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}

		return sender.Send(&backend.CallResourceResponse{
			Status: http.StatusOK,
			Headers: map[string][]string{"Content-Type": {"application/json"}},
			Body:   body,
		})
	})

	require.NoError(t, testEnv.PluginRegistry.Add(ctx, plugin))

	for _, ds := range []struct {
		uid  string
		name string
	}{
		{uid: leftUID, name: "Left alignment datasource"},
		{uid: rightUID, name: "Right alignment datasource"},
	} {
		_, err := testEnv.Server.HTTPServer.DataSourcesService.AddDataSource(ctx, &datasources.AddDataSourceCommand{
			OrgID:          1,
			Access:         datasources.DS_ACCESS_PROXY,
			Name:           ds.name,
			Type:           pluginID,
			UID:            ds.uid,
			URL:            "http://alignment.invalid",
			JsonData:       simplejson.New(),
			SecureJsonData: map[string]string{},
		})
		require.NoError(t, err)
	}

	t.Run("QueryData aligns mixed datasource frames on a shared timeline", func(t *testing.T) {
		requestBody := metricRequestWithQueries(t, fmt.Sprintf(`{
			"refId": "A",
			"intervalMs": 1000,
			"maxDataPoints": 1000000,
			"datasource": {
				"uid": %q,
				"type": %q
			}
		}`, leftUID, pluginID), fmt.Sprintf(`{
			"refId": "B",
			"intervalMs": 1000,
			"maxDataPoints": 1000000,
			"datasource": {
				"uid": %q,
				"type": %q
			}
		}`, rightUID, pluginID))

		resp := executeQueryDataRequest(t, grafanaListeningAddr, requestBody)
		defer func() {
			require.NoError(t, resp.Body.Close())
		}()

		var body queryAPIResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.ElementsMatch(t, []float64{1000, 2000, 3000, 4000}, body.Results["A"].Frames[0].Data.Values[0])
		require.ElementsMatch(t, []any{1.0, nil, 3.0, nil}, body.Results["A"].Frames[0].Data.Values[1])
		require.ElementsMatch(t, []float64{1000, 2000, 3000, 4000}, body.Results["B"].Frames[0].Data.Values[0])
		require.ElementsMatch(t, []any{nil, 20.0, 30.0, 40.0}, body.Results["B"].Frames[0].Data.Values[1])
	})

	t.Run("CallResource exposes the same aligned merge contract for incremental hydration", func(t *testing.T) {
		resourceURL := fmt.Sprintf("http://admin:admin@%s/api/datasources/uid/%s/resources/alignment", grafanaListeningAddr, leftUID)
		resourceReq, err := http.NewRequest(http.MethodPost, resourceURL, bytes.NewBufferString(`{"otherDatasourceUID":"mixed-right"}`))
		require.NoError(t, err)
		resourceReq.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(resourceReq)
		require.NoError(t, err)
		defer func() {
			require.NoError(t, resp.Body.Close())
		}()

		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)

		var payload struct {
			Rows []alignedRow `json:"rows"`
		}
		require.NoError(t, json.Unmarshal(body, &payload))

		require.Equal(t, http.StatusOK, resp.StatusCode)
		require.Equal(t, alignmentRows, payload.Rows)
	})
}

type alignedRow struct {
	Timestamp int64    `json:"timestamp"`
	Left      *float64 `json:"left,omitempty"`
	Right     *float64 `json:"right,omitempty"`
}

type queryAPIResponse struct {
	Results map[string]struct {
		Frames []struct {
			Data struct {
				Values [][]any `json:"values"`
			} `json:"data"`
		} `json:"frames"`
	} `json:"results"`
}

func executeQueryDataRequest(t *testing.T, grafanaListeningAddr string, body any) *http.Response {
	t.Helper()

	buf := &bytes.Buffer{}
	require.NoError(t, json.NewEncoder(buf).Encode(body))

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("http://admin:admin@%s/api/ds/query", grafanaListeningAddr), buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	return resp
}

func buildAlignedFrame(refID string, datasourceUID string, rows []alignedRow) *data.Frame {
	times := make([]time.Time, 0, len(rows))
	values := make([]*float64, 0, len(rows))

	for _, row := range rows {
		times = append(times, time.UnixMilli(row.Timestamp).UTC())
		switch datasourceUID {
		case "mixed-left":
			values = append(values, row.Left)
		default:
			values = append(values, row.Right)
		}
	}

	frame := data.NewFrame(refID,
		data.NewField("time", nil, times),
		data.NewField(refID, nil, values),
	)
	frame.RefID = refID
	return frame
}

func float64Pointer(value float64) *float64 {
	return &value
}
