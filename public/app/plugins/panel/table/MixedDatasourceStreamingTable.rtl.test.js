const React = require('react');
const { act, render, screen, waitFor } = require('@testing-library/react');
const { Provider, useDispatch, useSelector } = require('react-redux');
const { Subject } = require('rxjs');
const { createStore } = require('redux');

const initialState = {
  rows: {},
  visibleStart: 0,
  visibleCount: 2,
};

function mixedPanelReducer(state = initialState, action) {
  switch (action.type) {
    case 'applyChunk': {
      const nextRows = { ...state.rows };

      for (const point of action.payload.points) {
        const row = nextRows[point.timestamp] ?? {
          timestamp: point.timestamp,
          left: null,
          right: null,
        };

        nextRows[point.timestamp] = {
          ...row,
          left: action.payload.datasourceUID === 'left' ? point.value : row.left,
          right: action.payload.datasourceUID === 'right' ? point.value : row.right,
        };
      }

      return {
        ...state,
        rows: nextRows,
      };
    }
    case 'setVisibleRange':
      return {
        ...state,
        visibleStart: action.payload.start,
        visibleCount: action.payload.count,
      };
    default:
      return state;
  }
}

function selectSortedRows(state) {
  return Object.values(state.mixedPanel.rows).sort((left, right) => left.timestamp - right.timestamp);
}

function MixedDatasourceStreamingTable({ stream }) {
  const dispatch = useDispatch();
  const rows = useSelector(selectSortedRows);
  const { visibleStart, visibleCount } = useSelector((state) => state.mixedPanel);

  React.useEffect(() => {
    const subscription = stream.subscribe((chunk) => {
      dispatch({ type: 'applyChunk', payload: chunk });
    });

    return () => {
      subscription.unsubscribe();
    };
  }, [dispatch, stream]);

  const visibleRows = rows.slice(visibleStart, visibleStart + visibleCount);

  return React.createElement(
    'div',
    null,
    React.createElement('div', { 'data-testid': 'merged-row-count' }, rows.length),
    React.createElement(
      'div',
      { 'data-testid': 'visible-window' },
      visibleRows.map((row) =>
        React.createElement('div', { 'data-testid': 'visible-row', key: row.timestamp }, `${row.timestamp}|${row.left ?? '-'}|${row.right ?? '-'}`)
      )
    )
  );
}

describe('MixedDatasourceStreamingTable', () => {
  it('merges RxJS stream chunks on timestamp boundaries and reacts to Redux window changes', async () => {
    const stream = new Subject();
    const store = createStore((state, action) => ({
      mixedPanel: mixedPanelReducer(state?.mixedPanel, action),
    }));

    render(
      React.createElement(
        Provider,
        { store },
        React.createElement(MixedDatasourceStreamingTable, { stream })
      )
    );

    act(() => {
      stream.next({
        datasourceUID: 'left',
        points: [
          { timestamp: 1000, value: 1 },
          { timestamp: 3000, value: 3 },
        ],
      });
      stream.next({
        datasourceUID: 'right',
        points: [
          { timestamp: 2000, value: 20 },
          { timestamp: 3000, value: 30 },
          { timestamp: 4000, value: 40 },
        ],
      });
    });

    await waitFor(() => {
      expect(screen.getByTestId('merged-row-count')).toHaveTextContent('4');
    });

    expect(screen.getAllByTestId('visible-row')).toHaveLength(2);
    expect(screen.getByText('1000|1|-')).toBeInTheDocument();
    expect(screen.getByText('2000|-|20')).toBeInTheDocument();
    expect(screen.queryByText('3000|3|30')).not.toBeInTheDocument();

    act(() => {
      store.dispatch({ type: 'setVisibleRange', payload: { start: 2, count: 2 } });
    });

    await waitFor(() => {
      expect(screen.getByText('3000|3|30')).toBeInTheDocument();
      expect(screen.getByText('4000|-|40')).toBeInTheDocument();
    });

    expect(screen.queryByText('1000|1|-')).not.toBeInTheDocument();
    expect(screen.queryByText('2000|-|20')).not.toBeInTheDocument();
  });
});
