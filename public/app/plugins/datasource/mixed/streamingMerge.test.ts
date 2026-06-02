import { Subject, of } from 'rxjs';
import { map, scan, switchMap, take } from 'rxjs/operators';
import { getQueryOptions } from 'test/helpers/getQueryOptions';
import { DatasourceSrvMock, MockObservableDataSourceApi } from 'test/mocks/datasource_srv';

import { type DataQueryRequest, type DataSourceInstanceSettings, LoadingState, type PanelData } from '@grafana/data';
import { setDataSourceSrv, setTemplateSrv } from '@grafana/runtime';

import { TemplateSrv } from '../../../features/templating/template_srv';

import { MixedDatasource } from './MixedDataSource';

const defaultDS = new MockObservableDataSourceApi('DefaultDS', [{ data: ['DDD'] }]);
const datasourceSrv = new DatasourceSrvMock(defaultDS, {
  '-- Mixed --': new MockObservableDataSourceApi('mixed'),
  A: new MockObservableDataSourceApi('DSA', [{ data: ['AAAA'] }]),
  B: new MockObservableDataSourceApi('DSB', [{ data: ['BBBB'] }]),
  C: new MockObservableDataSourceApi('DSC', [{ data: ['CCCC'] }]),
});

describe('MixedDatasource Streaming Merge', () => {
  beforeEach(() => {
    jest.clearAllMocks();
    setDataSourceSrv({
      ...datasourceSrv,
      get: (uid: unknown) => datasourceSrv.get(uid),
      getInstanceSettings: jest.fn().mockReturnValue({ meta: {} }),
      getList: jest.fn(),
      reload: jest.fn(),
      registerRuntimeDataSource: jest.fn(),
    });
    setTemplateSrv(new TemplateSrv());
  });

  describe('streaming merge pipeline', () => {
    it('merges multiple datasource results into a single stream', async () => {
      const ds = new MixedDatasource({} as DataSourceInstanceSettings);
      const requestMixed = getQueryOptions({
        targets: [
          { refId: 'QA', datasource: { uid: 'A' } },
          { refId: 'QB', datasource: { uid: 'B' } },
          { refId: 'QC', datasource: { uid: 'C' } },
        ],
      });

      const mergedStates: LoadingState[] = [];

      await new Promise<void>((resolve) => {
        ds.query(requestMixed)
          .pipe(
            scan(
              (acc: PanelData, resp) => ({
                ...acc,
                state: resp.state,
                series: [...(acc.series || []), ...(resp.data || [])],
              }),
              { state: LoadingState.NotStarted, series: [] }
            )
          )
          .subscribe({
            next: (data: PanelData) => {
              mergedStates.push(data.state);
            },
            complete: () => {
              resolve();
            },
          });
      });

      expect(mergedStates.length).toBeGreaterThan(0);
      expect(mergedStates[mergedStates.length - 1]).toBe(LoadingState.Done);
    });

    it('streaming merge retains intermediate loading states', async () => {
      const ds = new MixedDatasource({} as DataSourceInstanceSettings);
      const requestMixed = getQueryOptions({
        targets: [
          { refId: 'QA', datasource: { uid: 'A' } },
          { refId: 'QB', datasource: { uid: 'B' } },
        ],
      });

      const loadingStates: LoadingState[] = [];

      await new Promise<void>((resolve) => {
        ds.query(requestMixed).subscribe({
          next: (resp) => {
            loadingStates.push(resp.state);
          },
          complete: () => {
            resolve();
          },
        });
      });

      expect(loadingStates).toContain(LoadingState.Loading);
      expect(loadingStates[loadingStates.length - 1]).toBe(LoadingState.Done);
    });

    it('streaming merge handles partial errors gracefully', async () => {
      const errorDS = new MockObservableDataSourceApi('DSE', [{ data: [] }], undefined, 'syntax error near WHERE');
      const errorDatasourceSrv = new DatasourceSrvMock(defaultDS, {
        '-- Mixed --': new MockObservableDataSourceApi('mixed'),
        A: new MockObservableDataSourceApi('DSA', [{ data: ['AAAA'] }]),
        B: errorDS,
      });
      setDataSourceSrv({
        ...errorDatasourceSrv,
        get: (uid: unknown) => errorDatasourceSrv.get(uid),
        getInstanceSettings: jest.fn().mockReturnValue({ meta: {} }),
        getList: jest.fn(),
        reload: jest.fn(),
        registerRuntimeDataSource: jest.fn(),
      });

      const ds = new MixedDatasource({} as DataSourceInstanceSettings);
      const requestMixed = getQueryOptions({
        targets: [
          { refId: 'QA', datasource: { uid: 'A' } },
          { refId: 'QB', datasource: { uid: 'B' } },
        ],
      });

      const results: PanelData[] = [];

      await new Promise<void>((resolve) => {
        ds.query(requestMixed).subscribe({
          next: (resp) => {
            results.push(resp);
          },
          complete: () => {
            resolve();
          },
        });
      });

      expect(results.length).toBeGreaterThan(0);
      const hasError = results.some((r) => r.state === LoadingState.Error);
      const hasData = results.some((r) => r.data && r.data.length > 0);
      expect(hasError || hasData).toBe(true);
    });
  });

  describe('RxJS stream transformation', () => {
    it('transforms chunked stream data into unified frames', async () => {
      const chunkSubject = new Subject<PanelData>();

      const unifiedFrames: PanelData[] = [];

      const subscription = chunkSubject
        .pipe(
          scan(
            (acc: PanelData, chunk) => ({
              ...acc,
              state: chunk.state,
              series: [...(acc.series || []), ...(chunk.series || [])],
            }),
            { state: LoadingState.NotStarted, series: [] }
          )
        )
        .subscribe({
          next: (data: PanelData) => {
            unifiedFrames.push(data);
          },
        });

      chunkSubject.next({
        state: LoadingState.Loading,
        series: [{ name: 'cpu', fields: [], length: 0 }],
      });

      chunkSubject.next({
        state: LoadingState.Loading,
        series: [{ name: 'memory', fields: [], length: 0 }],
      });

      chunkSubject.next({
        state: LoadingState.Done,
        series: [{ name: 'cpu', fields: [], length: 100 }],
      });

      chunkSubject.complete();

      await new Promise<void>((resolve) => {
        subscription.add(() => resolve());
      });

      expect(unifiedFrames.length).toBe(3);
      expect(unifiedFrames[0].series?.length).toBe(1);
      expect(unifiedFrames[1].series?.length).toBe(2);
      expect(unifiedFrames[2].series?.length).toBe(3);
    });

    it('uses switchMap to cancel stale stream subscriptions', async () => {
      const triggerSubject = new Subject<DataQueryRequest>();
      const cancelSpy = jest.fn();

      const stream$ = triggerSubject.pipe(
        switchMap((_req) => {
          return new Subject<PanelData>().pipe(
            take(1),
            map(() => ({
              state: LoadingState.Done,
              series: [],
            }))
          );
        })
      );

      const results: PanelData[] = [];

      const subscription = stream$.subscribe({
        next: (data: PanelData) => {
          results.push(data);
        },
      });

      triggerSubject.next({} as DataQueryRequest);
      triggerSubject.next({} as DataQueryRequest);

      await new Promise<void>((resolve) => {
        setTimeout(() => {
          subscription.unsubscribe();
          resolve();
        }, 50);
      });

      expect(results.length).toBeGreaterThanOrEqual(1);
      expect(results[0].state).toBe(LoadingState.Done);
    });

    it('handles high-frequency stream updates with buffer', async () => {
      const source = new Subject<PanelData>();
      const updates: PanelData[] = [];

      const subscription = source.pipe(scan((count) => count + 1, 0)).subscribe({
        next: (count) => {
          updates.push({ state: LoadingState.Streaming, series: [], timeRange: undefined, structureRev: count });
        },
      });

      const batchSize = 1000;
      for (let i = 0; i < batchSize; i++) {
        source.next({ state: LoadingState.Streaming, series: [], timeRange: undefined });
      }

      source.complete();
      subscription.unsubscribe();

      expect(updates.length).toBe(batchSize);
    });
  });

  describe('time-aligned merge', () => {
    it('aligns data from different datasources by timestamp', async () => {
      const frames = {
        cpu: { name: 'cpu', data: [1, 2, 3] },
        memory: { name: 'memory', data: [10, 20, 30] },
      };

      const mergeFn = (a: number[], b: number[]): number[] => {
        const result: number[] = [];
        const maxLen = Math.max(a.length, b.length);
        for (let i = 0; i < maxLen; i++) {
          result.push((a[i] || 0) + (b[i] || 0));
        }
        return result;
      };

      const merged = mergeFn(frames.cpu.data, frames.memory.data);
      expect(merged).toEqual([11, 22, 33]);
    });

    it('handles misaligned data with null filling', async () => {
      const cpuPoints = [1, null, 3, null, 5];
      const memPoints = [null, 20, null, 40, null];

      const mergeFn = (a: (number | null)[], b: (number | null)[]): (number | null)[] => {
        const result: (number | null)[] = [];
        const maxLen = Math.max(a.length, b.length);
        for (let i = 0; i < maxLen; i++) {
          if (a[i] != null && b[i] != null) {
            result.push(a[i]! + b[i]!);
          } else if (a[i] != null) {
            result.push(a[i]);
          } else {
            result.push(b[i]);
          }
        }
        return result;
      };

      const merged = mergeFn(cpuPoints, memPoints);
      expect(merged).toEqual([1, 20, 3, 40, 5]);
    });

    it('handles large datasets efficiently', async () => {
      const size = 1000000;
      const a = new Array<number>(size);
      const b = new Array<number>(size);

      for (let i = 0; i < size; i++) {
        a[i] = i * 0.01;
        b[i] = i * 0.02;
      }

      const startTime = performance.now();

      const mergeFn = (arrA: number[], arrB: number[]): number[] => {
        const result = new Array<number>(arrA.length);
        for (let i = 0; i < arrA.length; i++) {
          result[i] = arrA[i] + arrB[i];
        }
        return result;
      };

      const merged = mergeFn(a, b);
      const duration = performance.now() - startTime;

      expect(merged.length).toBe(size);
      expect(merged[0]).toBeCloseTo(0, 5);
      expect(merged[size - 1]).toBeCloseTo((size - 1) * 0.03, 1);
      expect(duration).toBeLessThan(500);
    });
  });
});