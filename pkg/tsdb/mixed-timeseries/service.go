package mixedtimeseries

import (
	"context"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	sdkhttpclient "github.com/grafana/grafana-plugin-sdk-go/backend/httpclient"
)

type Service struct {
	httpClientProvider *sdkhttpclient.Provider
}

func ProvideService(httpClientProvider *sdkhttpclient.Provider) *Service {
	return &Service{
		httpClientProvider: httpClientProvider,
	}
}

func (s *Service) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()

	for _, query := range req.Queries {
		frames, err := s.handleQuery(ctx, query)
		if err != nil {
			response.Responses[query.RefID] = backend.DataResponse{
				Error: err,
			}
			continue
		}

		aligned := timeAlign(frames)
		response.Responses[query.RefID] = backend.DataResponse{
			Frames: aligned,
		}
	}

	return response, nil
}

func (s *Service) handleQuery(ctx context.Context, query backend.DataQuery) (data.Frames, error) {
	return nil, nil
}
