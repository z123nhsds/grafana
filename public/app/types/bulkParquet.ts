import { type DataQuery, type DataSourceRef } from '@grafana/schema';
import { type Labels } from '@grafana/data';

export interface BulkParquetDataSourceOptions {
  inputPath?: string;
  batchSize?: number;
}

export interface BulkParquetQuery extends DataQuery {
  inputPath?: string;
  batchSize?: number;
  labelFilters?: LabelFilter[];
  datasourceRef?: DataSourceRef;
}

export interface LabelFilter {
  key: string;
  value: string;
  operator: LabelOperator;
}

export enum LabelOperator {
  Equal = '=',
  NotEqual = '!=',
  RegexMatch = '=~',
  RegexNotMatch = '!=~',
}

export interface BulkParquetRequest {
  queries: BulkParquetQuery[];
  requestId?: string;
  timezone?: string;
  app?: string;
  startTime?: number;
  endTime?: number;
  mode?: 'live' | 'streaming';
}

export interface BulkParquetResponse {
  data: BulkParquetResult[];
  errors?: BulkParquetError[];
}

export interface BulkParquetResult {
  frames: BulkParquetFrame[];
  key: string;
}

export interface BulkParquetFrame {
  schema: BulkParquetSchema;
  data: BulkParquetData;
}

export interface BulkParquetSchema {
  name?: string;
  fields: BulkParquetField[];
  meta?: Record<string, unknown>;
}

export interface BulkParquetField {
  name: string;
  type: BulkParquetFieldType;
  labels?: Labels;
  config?: Record<string, unknown>;
}

export type BulkParquetFieldType =
  | 'string'
  | 'number'
  | 'boolean'
  | 'enum'
  | 'time'
  | 'trace'
  | 'log'
  | 'structured metadata'
  | 'other';

export interface BulkParquetData {
  values: unknown[][];
  length: number;
}

export interface BulkParquetError {
  message: string;
  code?: string;
  details?: Record<string, unknown>;
}

export interface MixedBulkParquetQuery extends DataQuery {
  sources: BulkParquetQuery[];
  labelFilters?: LabelFilter[];
}

export interface StreamingBulkParquetOptions {
  inputPaths: string[];
  labelFilters?: LabelFilter[];
  batchSize?: number;
  refId?: string;
  datasource?: DataSourceRef;
}

export interface ExploreLogsBulkParquetState {
  labelFilters: LabelFilter[];
  isStreaming: boolean;
  visibleRange?: { start: number; end: number };
}

export const BULK_PARQUET_DATASOURCE_TYPE = 'grafana-bulk-parquet';

export const BULK_PARQUET_DEFAULT_BATCH_SIZE = 1024;

export function createLabelFilter(key: string, value: string, operator: LabelOperator = LabelOperator.Equal): LabelFilter {
  return { key, value, operator };
}

export function matchesLabelFilter(labels: Labels, filter: LabelFilter): boolean {
  const labelValue = labels[filter.key];
  if (labelValue === undefined) {
    return false;
  }

  switch (filter.operator) {
    case LabelOperator.Equal:
      return labelValue === filter.value;
    case LabelOperator.NotEqual:
      return labelValue !== filter.value;
    case LabelOperator.RegexMatch:
      try {
        const regex = new RegExp(filter.value);
        return regex.test(labelValue);
      } catch {
        return false;
      }
    case LabelOperator.RegexNotMatch:
      try {
        const regex = new RegExp(filter.value);
        return !regex.test(labelValue);
      } catch {
        return true;
      }
    default:
      return false;
  }
}

export function matchesAllLabelFilters(labels: Labels, filters: LabelFilter[]): boolean {
  if (filters.length === 0) {
    return true;
  }
  return filters.every((filter) => matchesLabelFilter(labels, filter));
}