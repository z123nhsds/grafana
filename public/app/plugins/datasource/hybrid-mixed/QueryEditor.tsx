import { type ChangeEvent } from 'react';

import { type QueryEditorProps } from '@grafana/data';
import { InlineField, InlineFieldRow, Input, TextArea } from '@grafana/ui';

import { type HybridMixedDataSource } from './datasource';
import { defaultQuery, type HybridMixedQuery } from './types';

type Props = QueryEditorProps<HybridMixedDataSource, HybridMixedQuery>;

export function QueryEditor(props: Props) {
  const { query, onChange, onRunQuery } = props;
  const currentQuery = { ...defaultQuery, ...query };

  const onQueryFieldChange = (field: keyof HybridMixedQuery) => (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const nextValue = event.currentTarget.type === 'number' ? Number(event.currentTarget.value) : event.currentTarget.value;

    onChange({
      ...currentQuery,
      [field]: nextValue,
    });
    onRunQuery();
  };

  return (
    <>
      <InlineFieldRow>
        <InlineField label="Prometheus" labelWidth={18} grow>
          <TextArea
            value={currentQuery.prometheusExpr}
            rows={3}
            onChange={onQueryFieldChange('prometheusExpr')}
          />
        </InlineField>
      </InlineFieldRow>
      <InlineFieldRow>
        <InlineField label="Loki" labelWidth={18} grow>
          <TextArea value={currentQuery.lokiExpr} rows={3} onChange={onQueryFieldChange('lokiExpr')} />
        </InlineField>
      </InlineFieldRow>
      <InlineFieldRow>
        <InlineField label="Step (ms)" labelWidth={18}>
          <Input type="number" value={currentQuery.stepMs} onChange={onQueryFieldChange('stepMs')} width={24} />
        </InlineField>
        <InlineField label="Loki limit" labelWidth={18}>
          <Input
            type="number"
            value={currentQuery.lokiLimit}
            onChange={onQueryFieldChange('lokiLimit')}
            width={24}
          />
        </InlineField>
      </InlineFieldRow>
    </>
  );
}
