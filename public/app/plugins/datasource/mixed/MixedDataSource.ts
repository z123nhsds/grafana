import { cloneDeep, groupBy } from 'lodash';
import { forkJoin, from, merge, Observable, of } from 'rxjs';
import { catchError, map, mergeAll, mergeMap, reduce, toArray } from 'rxjs/operators';

import {
  type DataQuery,
  type DataQueryError,
  type DataQueryRequest,
  type DataQueryResponse,
  type TestDataSourceResponse,
  DataSourceApi,
  type DataSourceInstanceSettings,
  LoadingState,
  type ScopedVars,
} from '@grafana/data';
import { getDataSourceSrv, getTemplateSrv, toDataQueryError } from '@grafana/runtime';
import { type CustomFormatterVariable } from '@grafana/scenes';

import { SHARED_DASHBOARD_QUERY } from '../dashboard/constants';

export const MIXED_DATASOURCE_NAME = '-- Mixed --';
export const MIXED_REQUEST_PREFIX = 'mixed-';
export const mixedAggregateRequestId = (requestId?: string) => `${MIXED_REQUEST_PREFIX}aggregate-${requestId || ''}`;

export const mixedRequestId = (queryIdx: number, requestId?: string) =>
  `${MIXED_REQUEST_PREFIX}${queryIdx}-${requestId || ''}`;

export interface BatchedQueries {
  datasource: Promise<DataSourceApi>;
  queries: DataQuery[];
  scopedVars: ScopedVars;
}

export interface MixedStreamingBatchState {
  completed: boolean;
  response?: DataQueryResponse;
}

interface MixedStreamingState {
  batches: MixedStreamingBatchState[];
}

type MixedStreamingEvent =
  | { type: 'response'; batchIndex: number; response: DataQueryResponse }
  | { type: 'complete'; batchIndex: number }
  | { type: 'error'; batchIndex: number; error: DataQueryError };

export class MixedDatasource extends DataSourceApi<DataQuery> {
  constructor(instanceSettings: DataSourceInstanceSettings) {
    super(instanceSettings);
  }

  query(request: DataQueryRequest<DataQuery>): Observable<DataQueryResponse> {
    const queries = request.targets.filter((t: DataQuery) => {
      return t.datasource?.uid !== MIXED_DATASOURCE_NAME;
    });

    if (!queries.length) {
      return of({ data: [] });
    }

    const sets: { [key: string]: DataQuery[] } = groupBy(queries, 'datasource.uid');
    const batches: BatchedQueries[] = [];

    for (const key in sets) {
      if (key === SHARED_DASHBOARD_QUERY) {
        sets[key].forEach((query) => {
          batches.push(...this.getBatchesForQueries([query], request));
        });
      } else {
        batches.push(...this.getBatchesForQueries(sets[key], request));
      }
    }

    if (!batches.length) {
      return of({ data: [] });
    }

    if (request.liveStreaming) {
      return this.streamQueries(batches, request);
    }

    return this.batchQueries(batches, request);
  }

  private getBatchesForQueries(queries: DataQuery[], request: DataQueryRequest<DataQuery>) {
    const dsRef = queries[0].datasource;
    const batches: BatchedQueries[] = [];

    const datasourceUid = getTemplateSrv().replace(
      dsRef?.uid,
      request.scopedVars,
      (value: string | string[], variable: CustomFormatterVariable) => {
        if (!Array.isArray(value)) {
          return value;
        }

        for (const uid of value) {
          if (uid === 'default') {
            continue;
          }

          const dsSettings = getDataSourceSrv().getInstanceSettings(uid);

          batches.push({
            datasource: getDataSourceSrv().get(uid),
            queries: cloneDeep(queries),
            scopedVars: {
              ...request.scopedVars,
              [variable.name]: { value: uid, text: dsSettings?.name },
            },
          });
        }

        return '';
      }
    );

    if (datasourceUid !== '') {
      batches.push({
        datasource: getDataSourceSrv().get(datasourceUid),
        queries: cloneDeep(queries),
        scopedVars: {
          ...request.scopedVars,
        },
      });
    }

    return batches;
  }

  batchQueries(mixed: BatchedQueries[], request: DataQueryRequest<DataQuery>): Observable<DataQueryResponse> {
    const runningQueries = mixed.filter(this.isQueryable).map((query, i) =>
      from(query.datasource).pipe(
        mergeMap((api: DataSourceApi) => {
          const dsRequest = cloneDeep(request);
          dsRequest.requestId = mixedRequestId(i, dsRequest.requestId);
          dsRequest.targets = query.queries;
          dsRequest.scopedVars = query.scopedVars;

          return from(api.query(dsRequest)).pipe(
            map((response: DataQueryResponse) => {
              return {
                ...response,
                data: response.data || [],
                state: LoadingState.Loading,
                key: mixedRequestId(i, response.key),
              };
            }),
            toArray(),
            catchError((err: any) => {
              err = toDataQueryError(err);
              err.message = `${api.name}: ${err.message}`;

              return of<DataQueryResponse[]>([
                {
                  data: [],
                  state: LoadingState.Error,
                  error: err,
                  key: mixedRequestId(i, dsRequest.requestId),
                },
              ]);
            })
          );
        })
      )
    );

    return forkJoin(runningQueries).pipe(flattenResponses(), map(this.finalizeResponses), mergeAll());
  }

  private streamQueries(mixed: BatchedQueries[], request: DataQueryRequest<DataQuery>): Observable<DataQueryResponse> {
    const runningQueries = mixed.filter(this.isQueryable).map((query, batchIndex) =>
      from(query.datasource).pipe(
        mergeMap((api: DataSourceApi) => this.runStreamingBatch(api, query, request, batchIndex))
      )
    );

    if (runningQueries.length === 0) {
      return of({ data: [], key: mixedAggregateRequestId(request.requestId), state: LoadingState.Done });
    }

    return merge(...runningQueries).pipe(
      reduceStreamingState(runningQueries.length, request.requestId),
      map((state: MixedStreamingState) => this.toStreamingResponse(state, request.requestId))
    );
  }

  private runStreamingBatch(
    api: DataSourceApi,
    query: BatchedQueries,
    request: DataQueryRequest<DataQuery>,
    batchIndex: number
  ): Observable<MixedStreamingEvent> {
    return new Observable<MixedStreamingEvent>((observer: any) => {
      const dsRequest = cloneDeep(request);
      dsRequest.requestId = mixedRequestId(batchIndex, dsRequest.requestId);
      dsRequest.targets = query.queries;
      dsRequest.scopedVars = query.scopedVars;

      const subscription = from(api.query(dsRequest)).subscribe({
        next: (response: any) => {
          observer.next({
            type: 'response',
            batchIndex,
            response: {
              ...response,
              data: response.data || [],
            },
          });
        },
        error: (err: any) => {
          const dataQueryError = toDataQueryError(err);
          dataQueryError.message = `${api.name}: ${dataQueryError.message}`;
          observer.next({ type: 'error', batchIndex, error: dataQueryError });
          observer.complete();
        },
        complete: () => {
          observer.next({ type: 'complete', batchIndex });
          observer.complete();
        },
      });

      return () => subscription.unsubscribe();
    });
  }

  private toStreamingResponse(state: MixedStreamingState, requestId?: string): DataQueryResponse {
    const traceIds = new Set<string>();
    const errors: DataQueryError[] = [];
    const data = state.batches.flatMap((batch) => {
      if (batch.response?.traceIds) {
        batch.response.traceIds.forEach((traceId: string) => traceIds.add(traceId));
      }
      if (batch.response?.error) {
        errors.push(batch.response.error);
      }
      if (batch.response?.errors?.length) {
        errors.push(...batch.response.errors);
      }
      return batch.response?.data ?? [];
    });

    const hasStreamingResponse = state.batches.some((batch) => batch.response?.state === LoadingState.Streaming);
    const allCompleted = state.batches.every((batch) => batch.completed);
    const responseState = errors.length
      ? LoadingState.Error
      : hasStreamingResponse
        ? LoadingState.Streaming
        : allCompleted
          ? LoadingState.Done
          : LoadingState.Loading;

    return {
      data,
      key: mixedAggregateRequestId(requestId),
      state: responseState,
      error: errors[0],
      errors: errors.length > 0 ? errors : undefined,
      traceIds: traceIds.size > 0 ? Array.from(traceIds) : undefined,
    };
  }

  testDatasource(): Promise<TestDataSourceResponse> {
    return Promise.resolve({ message: '', status: '' });
  }

  private isQueryable(query: BatchedQueries): boolean {
    return query && Array.isArray(query.queries) && query.queries.length > 0;
  }

  private finalizeResponses(responses: DataQueryResponse[]): DataQueryResponse[] {
    const { length } = responses;

    if (length === 0) {
      return responses;
    }

    const error = responses.find((response) => response.state === LoadingState.Error);
    if (error) {
      responses.push(error);
    } else {
      responses[length - 1].state = LoadingState.Done;
    }

    return responses;
  }
}

function flattenResponses() {
  return reduce((all: DataQueryResponse[], current: DataQueryResponse[]) => {
    return current.reduce((innerAll, innerCurrent) => {
      innerAll.push.apply(innerAll, innerCurrent);
      return innerAll;
    }, all);
  }, []);
}

function reduceStreamingState(batchCount: number, requestId?: string) {
  return reduce((state: MixedStreamingState, event: MixedStreamingEvent) => {
    const batch = state.batches[event.batchIndex];
    if (!batch) {
      return state;
    }

    if (event.type === 'response') {
      state.batches[event.batchIndex] = {
        ...batch,
        response: {
          ...event.response,
          key: mixedAggregateRequestId(requestId),
        },
      };
      return state;
    }

    if (event.type === 'complete') {
      state.batches[event.batchIndex] = {
        ...batch,
        completed: true,
      };
      return state;
    }

    state.batches[event.batchIndex] = {
      completed: true,
      response: {
        data: [],
        error: event.error,
        key: mixedAggregateRequestId(requestId),
        state: LoadingState.Error,
      },
    };
    return state;
  }, createInitialStreamingState(batchCount));
}

function createInitialStreamingState(batchCount: number): MixedStreamingState {
  return {
    batches: Array.from({ length: batchCount }, () => ({ completed: false })),
  };
}
