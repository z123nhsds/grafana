export interface LabelFilter {
  key: string;
  operator: string;
  values: string[];
}

export interface ParquetQuery {
  queryType?: string;
  paths?: string[];
  filters?: LabelFilter[];
}

export interface MixedDatasourceRef {
  type: string;
  uid?: string;
  name?: string;
}

export interface MixedQuery {
  datasourceRefs?: string[];
  queries?: ParquetQuery[];
  combineResults?: boolean;
}

export interface ParquetDataSourceOptions {
  path?: string;
}

export const defaultQuery: Partial<ParquetQuery> = {
  queryType: 'logs',
  paths: [],
  filters: [],
};
