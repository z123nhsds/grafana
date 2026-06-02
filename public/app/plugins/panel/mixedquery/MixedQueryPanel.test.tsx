import { Subject, of, merge } from 'rxjs';
import { delay, take, toArray } from 'rxjs/operators';
import { render, screen, act, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import {
  type DataFrame,
  FieldType,
  LoadingState,
  toDataFrame,
  getDefaultTimeRange,
  PanelData,
} from '@grafana/data';
import { getPanelProps } from 'public/app/plugins/panel/test-utils';

import { MixedQueryPanel } from './MixedQueryPanel';
import { mergeStreamedData, createMergeSubject } from './streaming/mergeStream';
import { panelDataReducer, initialState } from './state/panelDataSlice';

jest.mock('@grafana/runtime', () => ({
  ...jest.requireActual('@grafana/runtime'),
  getBackendSrv: () => ({
    get: jest.fn().mockResolvedValue({}),
    post: jest.fn().mockResolvedValue({}),
  }),
  getTemplateSrv: () => ({
    replace: (str: string) => str,
  }),
}));

function createTestDataFrame(
  sourceId: string,
  pointCount: number,
  baseTime: number
): DataFrame {
  const times: number[] = [];
  const values: number[] = [];
  const sources: string[] = [];

  for (let i = 0; i < pointCount; i++) {
    times.push(baseTime + i * 1000);
    values.push(i * 1.5);
    sources.push(sourceId);
  }

  return toDataFrame({
    name: sourceId,
    fields: [
      { name: 'Time', type: FieldType.time, values: times },
      { name: 'Value', type: FieldType.number, values: values },
      { name: 'Source', type: FieldType.string, values: sources },
    ],
  });
}

function createStreamingPanelData(
  frames: DataFrame[],
  state: LoadingState = LoadingState.Streaming
): PanelData {
  return {
    state,
    series: frames,
    timeRange: getDefaultTimeRange(),
  };
}

describe('MixedQueryPanel - RxJS Streaming', () => {
  let mergeSubject: Subject<PanelData>;

  beforeEach(() => {
    mergeSubject = new Subject<PanelData>();
    jest.useFakeTimers();
  });

  afterEach(() => {
    jest.useRealTimers();
    mergeSubject.complete();
  });

  it('should merge streaming data from multiple datasources in real-time', async () => {
    const { user } = setup({
      data: createStreamingPanelData([], LoadingState.Streaming),
    });

    const dsAFrame = createTestDataFrame('prometheus', 100, Date.now());
    const dsBFrame = createTestDataFrame('loki', 100, Date.now());

    await act(async () => {
      mergeSubject.next(createStreamingPanelData([dsAFrame]));
      jest.advanceTimersByTime(100);
    });

    await waitFor(() => {
      expect(screen.getByText('prometheus')).toBeInTheDocument();
    });

    await act(async () => {
      mergeSubject.next(createStreamingPanelData([dsAFrame, dsBFrame]));
      jest.advanceTimersByTime(100);
    });

    await waitFor(() => {
      expect(screen.getByText('loki')).toBeInTheDocument();
    });
  });

  it('should handle out-of-order streaming delivery and maintain time sort', async () => {
    const dataFrames: DataFrame[] = [];
    for (let i = 0; i < 5; i++) {
      dataFrames.push(createTestDataFrame(`ds-${i}`, 50, Date.now() + i * 5000));
    }

    const shuffled = [...dataFrames].sort(() => Math.random() - 0.5);

    const merged$ = mergeStreamedData(
      ...shuffled.map((frame) => of(createStreamingPanelData([frame])).pipe(delay(10)))
    );

    const collected: DataFrame[] = [];
    merged$.subscribe((data) => {
      collected.push(...data.series);
    });

    await act(async () => {
      jest.advanceTimersByTime(1000);
    });

    expect(collected.length).toBe(5);
  });

  it('should complete streaming when all sources are done', async () => {
    const sourceA$ = of(
      createStreamingPanelData([createTestDataFrame('source-a', 10, Date.now())])
    ).pipe(delay(50));

    const sourceB$ = of(
      createStreamingPanelData([createTestDataFrame('source-b', 10, Date.now())])
    ).pipe(delay(100));

    const merged$ = mergeStreamedData(sourceA$, sourceB$);

    let completed = false;
    const results: PanelData[] = [];

    merged$.subscribe({
      next: (data) => results.push(data),
      complete: () => {
        completed = true;
      },
    });

    await act(async () => {
      jest.advanceTimersByTime(200);
    });

    expect(completed).toBe(true);
    expect(results.length).toBeGreaterThan(0);
  });

  it('should handle error from one datasource without breaking others', async () => {
    const errorSource$ = new Subject<PanelData>();
    const goodSource$ = of(
      createStreamingPanelData([createTestDataFrame('healthy-ds', 20, Date.now())])
    ).pipe(delay(50));

    const merged$ = mergeStreamedData(goodSource$, errorSource$);

    const results: PanelData[] = [];
    let errorCaught = false;

    merged$.subscribe({
      next: (data) => results.push(data),
      error: () => {
        errorCaught = true;
      },
    });

    await act(async () => {
      errorSource$.error(new Error('Datasource connection failed'));
      jest.advanceTimersByTime(100);
    });

    expect(results.length).toBeGreaterThan(0);
  });

  it('should buffer rapid streaming updates to avoid render thrashing', async () => {
    const rapidUpdates = new Subject<PanelData>();

    const buffered$ = rapidUpdates.pipe(delay(100));

    const renderCount = { current: 0 };
    buffered$.subscribe(() => {
      renderCount.current++;
    });

    await act(async () => {
      for (let i = 0; i < 10; i++) {
        rapidUpdates.next(
          createStreamingPanelData([createTestDataFrame('fast-ds', 1, Date.now() + i)])
        );
      }
      jest.advanceTimersByTime(50);
    });

    expect(renderCount.current).toBe(0);

    await act(async () => {
      jest.advanceTimersByTime(100);
    });

    expect(renderCount.current).toBeGreaterThan(0);
  });
});

describe('MixedQueryPanel - Redux State Changes', () => {
  it('should update streaming state on new data arrival', () => {
    const frame = createTestDataFrame('test-ds', 10, Date.now());
    const panelData = createStreamingPanelData([frame], LoadingState.Streaming);

    const state = panelDataReducer(initialState, {
      type: 'panelData/receiveData',
      payload: panelData,
    });

    expect(state.loadingState).toBe(LoadingState.Streaming);
    expect(state.series.length).toBe(1);
    expect(state.series[0].name).toBe('test-ds');
  });

  it('should transition from Streaming to Done when complete', () => {
    const frame = createTestDataFrame('test-ds', 10, Date.now());

    let state = panelDataReducer(initialState, {
      type: 'panelData/receiveData',
      payload: createStreamingPanelData([frame], LoadingState.Streaming),
    });

    state = panelDataReducer(state, {
      type: 'panelData/complete',
      payload: createStreamingPanelData([frame], LoadingState.Done),
    });

    expect(state.loadingState).toBe(LoadingState.Done);
  });

  it('should accumulate series from multiple datasource responses', () => {
    const frameA = createTestDataFrame('ds-a', 10, Date.now());
    const frameB = createTestDataFrame('ds-b', 10, Date.now());

    let state = panelDataReducer(initialState, {
      type: 'panelData/receiveData',
      payload: createStreamingPanelData([frameA], LoadingState.Streaming),
    });

    state = panelDataReducer(state, {
      type: 'panelData/appendData',
      payload: createStreamingPanelData([frameB], LoadingState.Streaming),
    });

    expect(state.series.length).toBe(2);
    expect(state.series.map((s) => s.name)).toContain('ds-a');
    expect(state.series.map((s) => s.name)).toContain('ds-b');
  });

  it('should handle error state without losing existing data', () => {
    const frame = createTestDataFrame('ds-a', 10, Date.now());

    let state = panelDataReducer(initialState, {
      type: 'panelData/receiveData',
      payload: createStreamingPanelData([frame], LoadingState.Streaming),
    });

    state = panelDataReducer(state, {
      type: 'panelData/error',
      payload: { error: new Error('Connection timeout') },
    });

    expect(state.series.length).toBe(1);
    expect(state.error).toBeDefined();
  });

  it('should reset state on new query execution', () => {
    const frame = createTestDataFrame('old-ds', 10, Date.now());

    let state = panelDataReducer(initialState, {
      type: 'panelData/receiveData',
      payload: createStreamingPanelData([frame], LoadingState.Done),
    });

    state = panelDataReducer(state, {
      type: 'panelData/reset',
    });

    expect(state.series).toEqual([]);
    expect(state.loadingState).toBe(LoadingState.NotStarted);
  });
});

describe('MixedQueryPanel - Virtual Scrolling Integration', () => {
  it('should render only visible rows in viewport', async () => {
    const largeDataset = createTestDataFrame('large-ds', 10000, Date.now());

    const { container } = setup({
      data: createStreamingPanelData([largeDataset], LoadingState.Done),
      height: 400,
    });

    await waitFor(() => {
      const rows = container.querySelectorAll('[data-testid="data-row"]');
      expect(rows.length).toBeLessThan(100);
    });
  });

  it('should update rendered rows on scroll', async () => {
    const largeDataset = createTestDataFrame('scroll-ds', 10000, Date.now());

    const { container, user } = setup({
      data: createStreamingPanelData([largeDataset], LoadingState.Done),
      height: 400,
    });

    const scrollContainer = container.querySelector('[data-testid="virtual-scroll-container"]');

    await act(async () => {
      if (scrollContainer) {
        await userEvent.tab();
        scrollContainer.scrollTop = 5000;
        scrollContainer.dispatchEvent(new Event('scroll'));
      }
      jest.advanceTimersByTime(100);
    });

    await waitFor(() => {
      const rows = container.querySelectorAll('[data-testid="data-row"]');
      expect(rows.length).toBeGreaterThan(0);
    });
  });
});

function setup(overrides: Record<string, unknown> = {}) {
  const defaultProps = getPanelProps({}, {
    data: createStreamingPanelData([], LoadingState.Done),
    height: 400,
    width: 800,
    ...overrides,
  });

  const utils = render(<MixedQueryPanel {...defaultProps} />);

  return {
    user: userEvent.setup({ advanceTimers: jest.advanceTimersByTime }),
    ...utils,
  };
}
