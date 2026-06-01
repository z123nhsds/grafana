import { DataSourcePlugin } from '@grafana/data';
import { ParquetDatasource } from './datasource';
import { QueryEditor } from './QueryEditor';
import { ParquetDataSourceOptions, ParquetQuery } from './types';

export const plugin = new DataSourcePlugin<ParquetDatasource, ParquetQuery, ParquetDataSourceOptions>(
  ParquetDatasource
)
  .setQueryEditor(QueryEditor);
