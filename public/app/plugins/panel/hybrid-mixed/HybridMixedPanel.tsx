import { css } from '@emotion/css';
import AutoSizer, { type Size } from 'react-virtualized-auto-sizer';
import { FixedSizeList, type ListChildComponentProps } from 'react-window';
import { useEffect, useMemo, useRef, type MutableRefObject } from 'react';
import { Provider, useDispatch, useSelector } from 'react-redux';
import { filter, map } from 'rxjs/operators';

import {
  dateTimeFormat,
  type DataFrame,
  type GrafanaTheme2,
  type PanelProps,
  systemDateFormats,
} from '@grafana/data';
import { getBackendSrv } from '@grafana/runtime';
import { Alert, useStyles2 } from '@grafana/ui';

import {
  appendColumns,
  createHybridMixedPanelStore,
  replaceColumns,
  setError,
  setStreamStatus,
  type HybridMixedPanelRootState,
} from './state';
import { defaultPanelOptions, type HybridColumns, type HybridMixedPanelOptions } from './types';

interface StreamPoint {
  timestamp: number;
  prometheus: number | null;
  loki: number | null;
  merged: number | null;
}

interface StreamTarget {
  datasource?: { uid?: string };
  prometheusExpr?: string;
  lokiExpr?: string;
  stepMs?: number;
  lokiLimit?: number;
}

interface RowData {
  timestamps: number[];
  prometheus: Array<number | null>;
  loki: Array<number | null>;
  merged: Array<number | null>;
  styles: ReturnType<typeof getStyles>;
}

export function HybridMixedPanel(props: PanelProps<HybridMixedPanelOptions>) {
  const store = useMemo(() => createHybridMixedPanelStore(), []);

  return (
    <Provider store={store}>
      <HybridMixedPanelBody {...props} />
    </Provider>
  );
}

function HybridMixedPanelBody(props: PanelProps<HybridMixedPanelOptions>) {
  const options = { ...defaultPanelOptions, ...props.options };
  const styles = useStyles2(getStyles);
  const dispatch = useDispatch();
  const panelState = useSelector((state: HybridMixedPanelRootState) => state.panel);
  const remainderRef = useRef('');
  const requestTarget = getStreamTarget(props);
  const datasourceUid = requestTarget?.datasource?.uid ?? options.datasourceUid;
  const shouldStream = Boolean(options.liveUpdates && datasourceUid && (requestTarget?.prometheusExpr || requestTarget?.lokiExpr));

  useEffect(() => {
    if (shouldStream) {
      dispatch(replaceColumns(emptyColumns()));
      return;
    }

    dispatch(replaceColumns(frameToColumns(props.data.series[0])));
  }, [dispatch, props.data.series, shouldStream]);

  useEffect(() => {
    if (!shouldStream || !datasourceUid || !requestTarget) {
      dispatch(setStreamStatus('idle'));
      return;
    }

    const decoder = new TextDecoder();
    remainderRef.current = '';
    dispatch(setStreamStatus('streaming'));

    const subscription = getBackendSrv()
      .chunked({
        url: `/api/datasources/uid/${datasourceUid}/resources/stream`,
        params: {
          prometheusExpr: requestTarget.prometheusExpr ?? '',
          lokiExpr: requestTarget.lokiExpr ?? '',
          stepMs: `${requestTarget.stepMs ?? 1000}`,
          lokiLimit: `${requestTarget.lokiLimit ?? 1000}`,
          from: `${props.timeRange.from.valueOf()}`,
          to: `${props.timeRange.to.valueOf()}`,
          batchSize: `${options.batchSize}`,
          pushEveryMs: `${options.pushEveryMs}`,
        },
      })
      .pipe(
        filter((chunk: { ok: boolean; data?: Uint8Array }) => Boolean(chunk.ok && chunk.data)),
        map((chunk: { data?: Uint8Array }) => decodeChunk(decoder, remainderRef, chunk.data ?? new Uint8Array()))
      )
      .subscribe({
        next: (columns: HybridColumns) => {
          if (columns.timestamps.length) {
            dispatch(appendColumns(columns));
          }
        },
        error: (error: unknown) => {
          dispatch(setError(error instanceof Error ? error.message : 'Streaming request failed'));
        },
        complete: () => {
          dispatch(setStreamStatus('idle'));
        },
      });

    return () => {
      subscription.unsubscribe();
    };
  }, [
    datasourceUid,
    dispatch,
    options.batchSize,
    options.pushEveryMs,
    props.timeRange.from,
    props.timeRange.to,
    requestTarget,
    shouldStream,
  ]);

  if (!props.data.series.length && panelState.timestamps.length === 0) {
    return <Alert title="Hybrid Mixed" severity="info">No aligned data available for the current query.</Alert>;
  }

  return (
    <div className={styles.wrapper}>
      <div className={styles.summary}>
        <span>Rows: {panelState.timestamps.length.toLocaleString()}</span>
        <span>Status: {panelState.streamStatus}</span>
        <span>Range start: {dateTimeFormat(props.timeRange.from.valueOf(), { format: systemDateFormats.fullDate })}</span>
      </div>
      {panelState.error && <Alert title="Streaming error" severity="error">{panelState.error}</Alert>}
      <div className={styles.header}>
        <span>Time</span>
        <span>Prometheus</span>
        <span>Loki</span>
        <span>Merged</span>
      </div>
      <div className={styles.listWrapper}>
        <AutoSizer>
          {(size: Size) => renderVirtualList(size, panelState, styles, options.rowHeight)}
        </AutoSizer>
      </div>
    </div>
  );
}

function renderVirtualList(
  size: Size,
  panelState: HybridMixedPanelRootState['panel'],
  styles: ReturnType<typeof getStyles>,
  rowHeight: number
) {
  return (
    <FixedSizeList
      height={size.height}
      width={size.width}
      itemCount={panelState.timestamps.length}
      itemSize={rowHeight}
      itemData={{
        timestamps: panelState.timestamps,
        prometheus: panelState.prometheus,
        loki: panelState.loki,
        merged: panelState.merged,
        styles,
      }}
    >
      {VirtualRow}
    </FixedSizeList>
  );
}

function VirtualRow(props: ListChildComponentProps<RowData>) {
  const { index, style, data } = props;

  return (
    <div className={data.styles.row} style={style}>
      <span>{formatTimestamp(data.timestamps[index])}</span>
      <span>{formatNumeric(data.prometheus[index])}</span>
      <span>{formatNumeric(data.loki[index])}</span>
      <span>{formatNumeric(data.merged[index])}</span>
    </div>
  );
}

function getStreamTarget(props: PanelProps<HybridMixedPanelOptions>): StreamTarget | undefined {
  return props.data.request?.targets?.[0] as StreamTarget | undefined;
}

function frameToColumns(frame?: DataFrame): HybridColumns {
  if (!frame) {
    return emptyColumns();
  }

  const timestamps = getFieldValues<number | Date>(frame, 'time').map((value) =>
    value instanceof Date ? value.valueOf() : Number(value)
  );

  return {
    timestamps,
    prometheus: getFieldValues<number | null>(frame, 'prometheus').map(normalizeNullableNumber),
    loki: getFieldValues<number | null>(frame, 'loki').map(normalizeNullableNumber),
    merged: getFieldValues<number | null>(frame, 'merged').map(normalizeNullableNumber),
  };
}

function getFieldValues<T>(frame: DataFrame, name: string): T[] {
  const field = frame.fields.find((item) => item.name === name);
  if (!field) {
    return [];
  }

  return Array.from({ length: field.values.length }, (_, index) => field.values[index] as T);
}

function decodeChunk(decoder: TextDecoder, remainderRef: MutableRefObject<string>, chunk: Uint8Array): HybridColumns {
  const text = remainderRef.current + decoder.decode(chunk, { stream: true });
  const lines = text.split('\n');
  remainderRef.current = lines.pop() ?? '';

  const columns = emptyColumns();
  for (const line of lines) {
    if (!line) {
      continue;
    }

    const point = JSON.parse(line) as StreamPoint;
    columns.timestamps.push(point.timestamp);
    columns.prometheus.push(normalizeNullableNumber(point.prometheus));
    columns.loki.push(normalizeNullableNumber(point.loki));
    columns.merged.push(normalizeNullableNumber(point.merged));
  }

  return columns;
}

function emptyColumns(): HybridColumns {
  return {
    timestamps: [],
    prometheus: [],
    loki: [],
    merged: [],
  };
}

function normalizeNullableNumber(value: number | null | undefined): number | null {
  return value == null ? null : Number(value);
}

function formatTimestamp(value: number): string {
  return dateTimeFormat(value, { format: systemDateFormats.fullDate });
}

function formatNumeric(value: number | null): string {
  return value == null ? '—' : value.toFixed(3);
}

const getStyles = (theme: GrafanaTheme2) => ({
  wrapper: css({
    height: '100%',
    display: 'grid',
    gridTemplateRows: 'auto auto 1fr',
    gap: theme.spacing(1),
  }),
  summary: css({
    display: 'flex',
    gap: theme.spacing(2),
    fontSize: theme.typography.bodySmall.fontSize,
  }),
  header: css({
    display: 'grid',
    gridTemplateColumns: '2fr 1fr 1fr 1fr',
    gap: theme.spacing(1),
    padding: theme.spacing(0.5, 1),
    borderBottom: `1px solid ${theme.colors.border.weak}`,
    fontWeight: theme.typography.fontWeightMedium,
  }),
  listWrapper: css({
    minHeight: 0,
  }),
  row: css({
    display: 'grid',
    gridTemplateColumns: '2fr 1fr 1fr 1fr',
    gap: theme.spacing(1),
    padding: theme.spacing(0.5, 1),
    borderBottom: `1px solid ${theme.colors.border.weak}`,
    alignItems: 'center',
    fontFamily: theme.typography.fontFamilyMonospace,
  }),
});
