package mixedquery

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/datasource"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

var (
	_ backend.QueryDataHandler    = (*Service)(nil)
	_ backend.CallResourceHandler = (*Service)(nil)
	_ backend.CheckHealthHandler  = (*Service)(nil)
)

type Service struct {
	im     instancemgmt.InstanceManager
	logger log.Logger
}

var logger = backend.NewLoggerWith("logger", "tsdb.mixedquery")

func getRunContext() (string, int, string) {
	pc := make([]uintptr, 10)
	runtime.Callers(2, pc)
	f := runtime.FuncForPC(pc[0])
	file, line := f.FileLine(pc[0])
	return file, line, f.Name()
}

func logEntrypoint() string {
	file, line, pathToFunction := getRunContext()
	parts := strings.Split(pathToFunction, "/")
	functionName := parts[len(parts)-1]
	return fmt.Sprintf("%s:%d[%s]", file, line, functionName)
}

func (s *Service) getInstance(ctx context.Context, pluginCtx backend.PluginContext) (*MixedQueryDatasource, error) {
	ctxLogger := s.logger.FromContext(ctx)
	i, err := s.im.Get(ctx, pluginCtx)
	if err != nil {
		ctxLogger.Error("Failed to get instance", "error", err, "pluginID", pluginCtx.PluginID, "function", logEntrypoint())
		return nil, err
	}
	in := i.(*MixedQueryDatasource)
	return in, nil
}

func ProvideService() *Service {
	return &Service{
		im:     datasource.NewInstanceManager(newInstanceSettings()),
		logger: logger,
	}
}

func newInstanceSettings() datasource.InstanceFactoryFunc {
	return func(ctx context.Context, settings backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
		return NewMixedQueryDatasource(ctx, settings)
	}
}

func (s *Service) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	ctxLogger := s.logger.FromContext(ctx)
	ctxLogger.Debug("Processing queries", "queryLength", len(req.Queries), "function", logEntrypoint())

	i, err := s.getInstance(ctx, req.PluginContext)
	if err != nil {
		return nil, err
	}

	data, err := i.QueryData(ctx, req)
	if err != nil {
		ctxLogger.Error("Received error from MixedQuery", "error", err, "function", logEntrypoint())
	} else {
		ctxLogger.Debug("All queries processed", "function", logEntrypoint())
	}
	return data, err
}

func (s *Service) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	ctxLogger := s.logger.FromContext(ctx)
	ctxLogger.Debug("Calling resource", "path", req.Path, "function", logEntrypoint())

	i, err := s.getInstance(ctx, req.PluginContext)
	if err != nil {
		return err
	}

	err = i.CallResource(ctx, req, sender)
	if err != nil {
		ctxLogger.Error("Failed to call resource", "error", err, "function", logEntrypoint())
	} else {
		ctxLogger.Debug("Resource called", "function", logEntrypoint())
	}
	return err
}

func (s *Service) CheckHealth(ctx context.Context, req *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	ctxLogger := s.logger.FromContext(ctx)
	ctxLogger.Debug("Checking health", "function", logEntrypoint())

	i, err := s.getInstance(ctx, req.PluginContext)
	if err != nil {
		return nil, err
	}

	check, err := i.CheckHealth(ctx, req)
	if err != nil {
		ctxLogger.Error("Health check failed", "error", err, "function", logEntrypoint())
	} else {
		ctxLogger.Debug("Health check succeeded", "function", logEntrypoint())
	}
	return check, err
}
