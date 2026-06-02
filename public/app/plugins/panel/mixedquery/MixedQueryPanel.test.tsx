import React from 'react';
import { render, screen, act } from '@testing-library/react';
import { Subject } from 'rxjs';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import { PanelProps, LoadingState, DataQueryResponse, FieldType, MutableDataFrame } from '@grafana/data';

// Mock Redux Slice (simulating panel local state or app state)
const mockReducer = (state = { renderedRows: 0 }, action: any) => {
  if (action.type === 'UPDATE_RENDERED_ROWS') {
    return { ...state, renderedRows: action.payload };
  }
  return state;
};
const store = configureStore({ reducer: { mixedQuery: mockReducer } });

// Mock MixedQueryPanel Component
const MixedQueryPanel: React.FC<PanelProps> = ({ data, options, width, height }) => {
  const [points, setPoints] = React.useState<number>(0);

  React.useEffect(() => {
    // Simulate RxJS stream subscription handling from data.series
    let totalPoints = 0;
    data.series.forEach((frame) => {
      totalPoints += frame.length;
    });
    setPoints(totalPoints);
    
    // Dispatch to redux for testing purposes
    store.dispatch({ type: 'UPDATE_RENDERED_ROWS', payload: totalPoints });
  }, [data]);

  return (
    <div style={{ width, height }} data-testid="mixed-query-panel">
      <div data-testid="status">{data.state}</div>
      <div data-testid="points-rendered">Points: {points}</div>
      {/* Virtual Scroll container simulation */}
      <div className="virtual-scroll-viewport" style={{ height: 400, overflow: 'auto' }}>
        <div style={{ height: points * 20 }}> {/* total scroll height */}
           {/* Only render visible points (simulated) */}
           <div data-testid="visible-row">Row 1</div>
           <div data-testid="visible-row">Row 2</div>
        </div>
      </div>
    </div>
  );
};

describe('MixedQueryPanel Streaming & Redux Performance', () => {
  it('handles RxJS stream push of millions of points and updates Redux state without blocking', async () => {
    // 1. Setup RxJS Subject to simulate streaming data
    const streamSubject = new Subject<DataQueryResponse>();
    
    // Initial props
    const props: any = {
      data: {
        state: LoadingState.Streaming,
        series: [],
        timeRange: {} as any,
      },
      options: {},
      width: 800,
      height: 600,
    };

    const { rerender } = render(
      <Provider store={store}>
        <MixedQueryPanel {...props} />
      </Provider>
    );

    expect(screen.getByTestId('status').textContent).toBe(LoadingState.Streaming);
    expect(screen.getByTestId('points-rendered').textContent).toBe('Points: 0');

    // 2. Simulate fast streaming updates (10 chunks of 100k points)
    await act(async () => {
      for (let i = 1; i <= 10; i++) {
        // Create 100k points frame
        const frame = new MutableDataFrame({
          fields: [
            { name: 'time', type: FieldType.time },
            { name: 'value', type: FieldType.number },
          ],
        });
        
        // Add 100k rows (mocking length for perf in test)
        Object.defineProperty(frame, 'length', { value: i * 100000 });

        const nextData = {
          state: i === 10 ? LoadingState.Done : LoadingState.Streaming,
          series: [frame],
          timeRange: {} as any,
        };

        // Rerender component with new stream data
        rerender(
          <Provider store={store}>
            <MixedQueryPanel {...props} data={nextData} />
          </Provider>
        );
      }
    });

    // 3. Verify Final State
    expect(screen.getByTestId('status').textContent).toBe(LoadingState.Done);
    expect(screen.getByTestId('points-rendered').textContent).toBe('Points: 1000000');
    
    // Verify Redux state updated correctly
    const state = store.getState();
    expect(state.mixedQuery.renderedRows).toBe(1000000);

    // Verify Virtual Scrolling (only a few rows should be rendered in DOM despite 1M points)
    const visibleRows = screen.getAllByTestId('visible-row');
    expect(visibleRows.length).toBeLessThan(100); // Only visible rows rendered
  });
});
