import { LoadingState } from './data';
import { DataQueryError, DataQueryResponse } from './datasource';

export interface ParquetLabelFilter {
  key: string;
  value: string;
  op: ParquetLabelFilterOp;
}

export enum ParquetLabelFilterOp {
  Equal = 0,
  NotEqual = 1,
}

export interface ParquetLogLine {
  timestamp: number;
  line: string;
  labels: Record<string, string>;
}

export interface ParquetLogQueryRequest {
  filePath: string;
  batchSize: number;
  filters: ParquetLabelFilter[];
}

export interface MixedStreamingBatchState {
  completed: boolean;
  response?: DataQueryResponse;
}

export interface MixedStreamingState {
  batches: MixedStreamingBatchState[];
}

export type MixedStreamingEvent =
  | { type: 'response'; batchIndex: number; response: DataQueryResponse }
  | { type: 'complete'; batchIndex: number }
  | { type: 'error'; batchIndex: number; error: DataQueryError };

export function mixedAggregateRequestId(requestId?: string): string {
  return `mixed-aggregate-${requestId || ''}`;
}

export function createInitialStreamingState(batchCount: number): MixedStreamingState {
  return {
    batches: Array.from({ length: batchCount }, () => ({ completed: false })),
  };
}

export function reduceStreamingState(
  requestId?: string
): (state: MixedStreamingState, event: MixedStreamingEvent) => MixedStreamingState {
  return (state: MixedStreamingState, event: MixedStreamingEvent): MixedStreamingState => {
    const batch = state.batches[event.batchIndex];
    if (!batch) {
      return state;
    }

    if (event.type === 'response') {
      return {
        ...state,
        batches: state.batches.map((b, i) =>
          i === event.batchIndex
            ? {
                ...b,
                response: {
                  ...event.response,
                  key: mixedAggregateRequestId(requestId),
                },
              }
            : b
        ),
      };
    }

    if (event.type === 'complete') {
      return {
        ...state,
        batches: state.batches.map((b, i) =>
          i === event.batchIndex ? { ...b, completed: true } : b
        ),
      };
    }

    return {
      ...state,
      batches: state.batches.map((b, i) =>
        i === event.batchIndex
          ? {
              completed: true,
              response: {
                data: [],
                error: event.error,
                key: mixedAggregateRequestId(requestId),
                state: LoadingState.Error,
              },
            }
          : b
      ),
    };
  };
}