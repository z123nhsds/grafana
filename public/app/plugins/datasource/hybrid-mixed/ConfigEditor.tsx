import { type ChangeEvent } from 'react';

import { type DataSourcePluginOptionsEditorProps } from '@grafana/data';
import { InlineField, InlineFieldRow, Input } from '@grafana/ui';

import { type HybridMixedDataSourceOptions } from './types';

export function ConfigEditor(props: DataSourcePluginOptionsEditorProps<HybridMixedDataSourceOptions>) {
  const { options, onOptionsChange } = props;

  const onJsonDataChange =
    (field: keyof HybridMixedDataSourceOptions) => (event: ChangeEvent<HTMLInputElement>) => {
      onOptionsChange({
        ...options,
        jsonData: {
          ...options.jsonData,
          [field]: event.currentTarget.value,
        },
      });
    };

  return (
    <>
      <InlineFieldRow>
        <InlineField label="Prometheus URL" labelWidth={20} grow>
          <Input
            value={options.jsonData.prometheusUrl ?? ''}
            placeholder="http://localhost:9090"
            onChange={onJsonDataChange('prometheusUrl')}
          />
        </InlineField>
      </InlineFieldRow>
      <InlineFieldRow>
        <InlineField label="Loki URL" labelWidth={20} grow>
          <Input
            value={options.jsonData.lokiUrl ?? ''}
            placeholder="http://localhost:3100"
            onChange={onJsonDataChange('lokiUrl')}
          />
        </InlineField>
      </InlineFieldRow>
    </>
  );
}
