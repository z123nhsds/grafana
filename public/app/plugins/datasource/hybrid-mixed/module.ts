import { DataSourcePlugin } from '@grafana/data';

import { ConfigEditor } from './ConfigEditor';
import { HybridMixedDataSource } from './datasource';
import { QueryEditor } from './QueryEditor';

export const plugin = new DataSourcePlugin(HybridMixedDataSource)
  .setConfigEditor(ConfigEditor)
  .setQueryEditor(QueryEditor);
