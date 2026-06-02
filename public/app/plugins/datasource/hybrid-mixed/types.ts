import { type DataQuery, type DataSourceJsonData } from '@grafana/data';

export interface HybridMixedQuery extends DataQuery {
  prometheusExpr: string;
  lokiExpr: string;
  stepMs: number;
  lokiLimit: number;
}

export interface HybridMixedDataSourceOptions extends DataSourceJsonData {
  prometheusUrl?: string;
  lokiUrl?: string;
}

export const defaultQuery: HybridMixedQuery = {
  refId: 'A',
  prometheusExpr: 'sum(rate(grafana_http_request_duration_seconds_count[$__rate_interval]))',
  lokiExpr: 'sum(count_over_time({job="grafana"}[$__interval]))',
  stepMs: 1000,
  lokiLimit: 1000,
};
