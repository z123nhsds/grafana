import {
  DataQueryRequest,
  DataQueryResponse,
  DataSourceApi,
  DataSourceInstanceSettings,
  MutableDataFrame,
  FieldType,
  Labels,
} from '@grafana/data';
import { getBackendSrv, getTemplateSrv } from '@grafana/runtime';
import { ParquetQuery, ParquetDataSourceOptions, defaultQuery, LabelFilter } from './types';

export class ParquetDatasource extends DataSourceApi<ParquetQuery, ParquetDataSourceOptions> {
  constructor(instanceSettings: DataSourceInstanceSettings<ParquetDataSourceOptions>) {
    super(instanceSettings);
  }

  async query(options: DataQueryRequest<ParquetQuery>): Promise<DataQueryResponse> {
    const { range, rangeRaw, interval, intervalMs, targets, scopedVars } = options;
    
    const queries = targets.filter(t => !t.hide).map(t => {
      const query = { ...defaultQuery, ...t };
      if (query.paths) {
        query.paths = query.paths.map(p => getTemplateSrv().replace(p, scopedVars));
      }
      return query;
    });

    if (queries.length === 0) {
      return { data: [] };
    }

    try {
      const response = await getBackendSrv().datasourceRequest({
        url: '/api/ds/query',
        method: 'POST',
        data: {
          queries,
          from: rangeRaw.from,
          to: rangeRaw.to,
        },
      });

      return response.data;
    } catch (error) {
      console.error('Query failed:', error);
      return { data: [] };
    }
  }

  async testDatasource(): Promise<{ status: string; message: string }> {
    try {
      await getBackendSrv().get(`/api/datasources/${this.id}/health`);
      return {
        status: 'success',
        message: 'Data source is working',
      };
    } catch (error) {
      console.error('Health check failed:', error);
      return {
        status: 'error',
        message: 'Data source is not responding',
      };
    }
  }

  getQueryDisplayText(query: ParquetQuery): string {
    if (query.paths && query.paths.length > 0) {
      return `Parquet: ${query.paths.join(', ')}`;
    }
    return 'Parquet Query';
  }
}
