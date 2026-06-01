import React, { useState } from 'react';
import { QueryEditorProps, InlineField, Input, Select, Button } from '@grafana/ui';
import { ParquetDatasource } from './datasource';
import { ParquetQuery, LabelFilter, defaultQuery } from './types';

interface Props extends QueryEditorProps<ParquetDatasource, ParquetQuery> {}

const operatorOptions = [
  { label: '=', value: '=' },
  { label: '!=', value: '!=' },
  { label: '=~', value: '=~' },
  { label: '!~', value: '!~' },
];

export const QueryEditor: React.FC<Props> = ({ query, onChange, onRunQuery }) => {
  const [paths, setPaths] = useState(query.paths || []);
  const [newPath, setNewPath] = useState('');
  const [filters, setFilters] = useState<LabelFilter[]>(query.filters || []);

  const onPathsChange = (paths: string[]) => {
    const updatedQuery = { ...query, paths };
    onChange(updatedQuery);
  };

  const addPath = () => {
    if (newPath.trim()) {
      const newPaths = [...paths, newPath.trim()];
      setPaths(newPaths);
      onPathsChange(newPaths);
      setNewPath('');
    }
  };

  const removePath = (index: number) => {
    const newPaths = [...paths];
    newPaths.splice(index, 1);
    setPaths(newPaths);
    onPathsChange(newPaths);
  };

  const onFiltersChange = (filters: LabelFilter[]) => {
    const updatedQuery = { ...query, filters };
    onChange(updatedQuery);
  };

  const addFilter = () => {
    const newFilters = [...filters, { key: '', operator: '=', values: [] }];
    setFilters(newFilters);
    onFiltersChange(newFilters);
  };

  const updateFilter = (index: number, updates: Partial<LabelFilter>) => {
    const newFilters = [...filters];
    newFilters[index] = { ...newFilters[index], ...updates };
    setFilters(newFilters);
    onFiltersChange(newFilters);
  };

  const removeFilter = (index: number) => {
    const newFilters = [...filters];
    newFilters.splice(index, 1);
    setFilters(newFilters);
    onFiltersChange(newFilters);
  };

  return (
    <div>
      <div className="gf-form-group">
        <h6>Paths</h6>
        {paths.map((path, idx) => (
          <div key={idx} className="gf-form-inline" style={{ marginBottom: '4px' }}>
            <Input
              value={path}
              readOnly
              style={{ width: '80%' }}
            />
            <Button
              variant="destructive"
              size="sm"
              onClick={() => removePath(idx)}
              style={{ marginLeft: '8px' }}
            >
              Remove
            </Button>
          </div>
        ))}
        <div className="gf-form-inline" style={{ marginTop: '8px' }}>
          <Input
            value={newPath}
            onChange={(e) => setNewPath(e.currentTarget.value)}
            placeholder="Add a file path"
            style={{ width: '80%' }}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                addPath();
              }
            }}
          />
          <Button
            variant="secondary"
            size="sm"
            onClick={addPath}
            style={{ marginLeft: '8px' }}
          >
            Add
          </Button>
        </div>
      </div>

      <div className="gf-form-group">
        <h6>Filters</h6>
        {filters.map((filter, idx) => (
          <div key={idx} className="gf-form-inline" style={{ marginBottom: '4px' }}>
            <InlineField label="Key">
              <Input
                value={filter.key}
                onChange={(e) => updateFilter(idx, { key: e.currentTarget.value })}
                placeholder="Label key"
              />
            </InlineField>
            <InlineField label="Operator">
              <Select
                options={operatorOptions}
                value={{ value: filter.operator, label: filter.operator }}
                onChange={(v) => updateFilter(idx, { operator: v.value })}
              />
            </InlineField>
            <InlineField label="Values">
              <Input
                value={filter.values.join(', ')}
                onChange={(e) => updateFilter(idx, { values: e.currentTarget.value.split(/\s*,\s*/) })}
                placeholder="Value1, Value2"
              />
            </InlineField>
            <Button
              variant="destructive"
              size="sm"
              onClick={() => removeFilter(idx)}
            >
              Remove
            </Button>
          </div>
        ))}
        <Button
          variant="secondary"
          size="sm"
          onClick={addFilter}
          style={{ marginTop: '8px' }}
        >
          Add Filter
        </Button>
      </div>
    </div>
  );
};
