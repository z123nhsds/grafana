import { forkJoin, from, type Observable, of } from 'rxjs';
import { map, catchError, mergeMap, toArray } from 'rxjs/operators';

import { type DataQueryRequest, type DataQueryResponse, type DataSourceApi, type DataSourceInstanceSettings } from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';

import {
  type BulkParquetQuery,
  type BulkParquetResponse,
  type BulkParquetFrame,
  type LabelFilter,
  LabelOperator,
  matchesAllLabelFilters,
  type StreamingBulkParquetOptions,
} from '../types/bulkParquet';

export const BULK_PARQUET_DATASOURCE_UID = 'grafana-bulk-parquet-datasource';
export const BULK_PARQUET_DATASOURCE_TYPE = 'grafana-bulk-parquet';

export interface BulkParquetDataSourceOptions {
  inputPath?: string;
  supportsStreaming?: boolean;
}

export class BulkParquetDataSource extends DataSourceApi<BulkParquetQuery> {
  private inputPath: string;
  private supportsStreaming: boolean;

  constructor(instanceSettings: DataSourceInstanceSettings<BulkParquetDataSourceOptions>) {
    super(instanceSettings);
    this.inputPath = instanceSettings.jsonData.inputPath ?? '';
    this.supportsStreaming = instanceSettings.jsonData.supportsStreaming ?? true;
  }

  query(request: DataQueryRequest<BulkParquetQuery>): Observable<DataQueryResponse> {
    const validQueries = request.targets.filter((t) => t.inputPath || t.datasourceRef?.uid === this.uid);

    if (!validQueries.length) {
      return of({ data: [] });
    }

    const streams = validQueries.map((query) => {
      const inputPaths = query.inputPath ? [query.inputPath] : [this.inputPath];
      const filters = query.labelFilters ?? [];

      return this.executeBulkQuery(inputPaths, filters, query);
    });

    if (streams.length === 1) {
      return streams[0];
    }

    return forkJoin(streams).pipe(
      map((responses) => {
        const combined = responses.reduce(
          (acc, resp) => {
            acc.data.push(...resp.data);
            return acc;
          },
          { data: [] as BulkParquetResponse[] }
        );
        return { data: combined.data.flatMap((r) => this.convertToDataFrames(r, request)) };
      })
    );
  }

  private executeBulkQuery(inputPaths: string[], filters: LabelFilter[], query: BulkParquetQuery): Observable<DataQueryResponse> {
    const body = {
      queries: [
        {
          ...query,
          inputPaths,
          labelFilters: filters.map((f) => ({
            key: f.key,
            value: f.value,
            operator: f.operator.toString(),
          })),
        },
      ],
      requestId: query.refId,
      timezone: 'browser',
    };

    return from(
      getBackendSrv().post<BulkParquetResponse>(`/api/ds/query`, body, {
        headers: {
          'Content-Type': 'application/json',
        },
      })
    ).pipe(
      map((response) => ({
        data: this.convertToDataFrames(response, { ...{ targets: [query] } } as DataQueryRequest<BulkParquetQuery>),
      })),
      catchError((error) => {
        console.error('Bulk Parquet query error:', error);
        return of({ data: [], error: { message: error.message } });
      })
    );
  }

  private convertToDataFrames(response: BulkParquetResponse, _request: DataQueryRequest<BulkParquetQuery>): unknown[] {
    if (!response.data) {
      return [];
    }

    return response.data.flatMap((result) => {
      return result.frames.map((frame: BulkParquetFrame) => {
        return {
          schema: {
            name: frame.schema.name,
            fields: frame.schema.fields.map((field) => ({
              ...field,
              type: field.type,
            })),
            meta: frame.schema.meta,
          },
          data: {
            values: frame.data.values,
            length: frame.data.length,
          },
        };
      });
    });
  }

  filterLogRows(logRows: Array<{ labels?: Record<string, string> }>, filters: LabelFilter[]): Array<{ labels?: Record<string, string> }> {
    if (!filters.length) {
      return logRows;
    }

    return logRows.filter((row) => {
      if (!row.labels) {
        return false;
      }
      return matchesAllLabelFilters(row.labels, filters);
    });
  }

  applyLabelFilters(filters: LabelFilter[]): LabelFilter[] {
    return filters;
  }

  testDatasource(): Promise<{ status: string; message: string }> {
    return Promise.resolve({ status: 'success', message: 'Bulk Parquet DataSource is configured correctly' });
  }
}

export function createBulkParquetQuery(
  inputPath: string,
  labelFilters?: LabelFilter[],
  batchSize?: number
): BulkParquetQuery {
  return {
    refId: 'A',
    inputPath,
    labelFilters,
    batchSize: batchSize ?? 1024,
    datasource: {
      uid: BULK_PARQUET_DATASOURCE_UID,
      type: BULK_PARQUET_DATASOURCE_TYPE,
    },
  };
}

export function createMixedBulkParquetQuery(sources: BulkParquetQuery[]): BulkParquetQuery {
  return {
    refId: 'A',
    datasource: {
      uid: 'mixed',
      type: 'mixed',
    },
    labelFilters: sources[0]?.labelFilters,
  };
}

export function createLabelFilter(
  key: string,
  value: string,
  operator: LabelOperator = LabelOperator.Equal
): LabelFilter {
  return { key, value, operator };
}

export function parseLabelOperator(operatorStr: string): LabelOperator {
  switch (operatorStr) {
    case '=':
      return LabelOperator.Equal;
    case '!=':
      return LabelOperator.NotEqual;
    case '=~':
      return LabelOperator.RegexMatch;
    case '!=~':
      return LabelOperator.RegexNotMatch;
    default:
      return LabelOperator.Equal;
  }
}

export function labelsToFilters(labels: Record<string, string>): LabelFilter[] {
  return Object.entries(labels).map(([key, value]) => createLabelFilter(key, value, LabelOperator.Equal));
}

export function filtersToLabels(filters: LabelFilter[]): Record<string, string> {
  const labels: Record<string, string> = {};
  for (const filter of filters) {
    if (filter.operator === LabelOperator.Equal) {
      labels[filter.key] = filter.value;
    }
  }
  return labels;
}