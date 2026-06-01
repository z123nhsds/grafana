import { type Labels } from '@grafana/data';

export type LabelOperatorType = '=' | '!=' | '=~' | '!=~';

export interface BulkParquetLabelFilter {
  key: string;
  value: string;
  op: LabelOperatorType;
}

export interface BulkParquetReaderConfig {
  inputPath: string;
  batchSize: number;
  labelFilters: BulkParquetLabelFilter[];
  mixedGroup?: string;
  mixedResource?: string;
}

export interface BulkParquetResourceKey {
  group: string;
  resource: string;
  namespace: string;
  name: string;
}

export interface BulkParquetRequest {
  key: BulkParquetResourceKey;
  action: number;
  value: Uint8Array;
  folder: string;
}

export interface BulkParquetResponse {
  processed: number;
  error?: {
    message: string;
  };
}

export interface BulkParquetLogEntry {
  labels: Labels;
  timestamp: number;
  line: string;
  namespace: string;
  group: string;
  resource: string;
  name: string;
}

export interface BulkParquetQuery {
  refId?: string;
  inputPath: string;
  labelFilters: BulkParquetLabelFilter[];
  batchSize?: number;
  startTime?: number;
  endTime?: number;
}

export interface BulkParquetStreamQuery {
  queries: BulkParquetQuery[];
  mixed?: boolean;
}

export interface BulkParquetQueryResult {
  data: BulkParquetLogEntry[];
  metadata?: {
    totalBytes?: number;
    processedRecords?: number;
    durationMs?: number;
  };
}

export type BulkParquetLabelFilterActiveType = (key: string, value: string, refId?: string) => Promise<boolean>;

export interface BulkParquetLogsPanelOptions {
  enableLogDetails?: boolean;
  showLabels?: boolean;
  showCommonLabels?: boolean;
  showTime?: boolean;
  wrapLogMessage?: boolean;
  prettifyLogMessage?: boolean;
  enableLogChronologicalOperation?: boolean;
  sortOrder?: 'Descending' | 'Ascending';
  dedupStrategy?: 'none' | 'exact' | 'numbers' | 'signature';
  isLabelFilterActive?: BulkParquetLabelFilterActiveType;
}

export function isBulkParquetLabelFilterActive(
  callback: unknown
): callback is BulkParquetLabelFilterActiveType {
  return typeof callback === 'function';
}

export function createLabelFilter(
  key: string,
  value: string,
  op: LabelOperatorType = '='
): BulkParquetLabelFilter {
  return { key, value, op };
}

export function createEqualLabelFilter(key: string, value: string): BulkParquetLabelFilter {
  return createLabelFilter(key, value, '=');
}

export function createNotEqualLabelFilter(key: string, value: string): BulkParquetLabelFilter {
  return createLabelFilter(key, value, '!=');
}

export function createRegexMatchLabelFilter(key: string, pattern: string): BulkParquetLabelFilter {
  return createLabelFilter(key, pattern, '=~');
}

export function createRegexNotMatchLabelFilter(key: string, pattern: string): BulkParquetLabelFilter {
  return createLabelFilter(key, pattern, '!=~');
}
