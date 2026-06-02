import { type Observable } from 'rxjs';

import {
  CoreApp,
  type DataQueryRequest,
  type DataQueryResponse,
  type DataSourceInstanceSettings,
  type ScopedVars,
} from '@grafana/data';
import { DataSourceWithBackend, getTemplateSrv } from '@grafana/runtime';

import { defaultQuery, type HybridMixedDataSourceOptions, type HybridMixedQuery } from './types';

export class HybridMixedDataSource extends DataSourceWithBackend<HybridMixedQuery, HybridMixedDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<HybridMixedDataSourceOptions>) {
    super(instanceSettings);
  }

  query(request: DataQueryRequest<HybridMixedQuery>): Observable<DataQueryResponse> {
    const targets = this.interpolateVariablesInQueries(request.targets, request.scopedVars);
    return super.query({ ...request, targets });
  }

  interpolateVariablesInQueries(queries: HybridMixedQuery[], scopedVars: ScopedVars): HybridMixedQuery[] {
    return queries.map((query) => this.applyTemplateVariables(query, scopedVars));
  }

  applyTemplateVariables(query: HybridMixedQuery, scopedVars: ScopedVars): HybridMixedQuery {
    const templateSrv = getTemplateSrv();

    return {
      ...defaultQuery,
      ...query,
      prometheusExpr: templateSrv.replace(query.prometheusExpr ?? '', scopedVars),
      lokiExpr: templateSrv.replace(query.lokiExpr ?? '', scopedVars),
    };
  }

  getDefaultQuery(_: CoreApp): Partial<HybridMixedQuery> {
    return defaultQuery;
  }

  getQueryDisplayText(query: HybridMixedQuery): string {
    return [query.prometheusExpr, query.lokiExpr].filter(Boolean).join(' | ');
  }
}
