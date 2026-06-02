import { css } from '@emotion/css';
import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { type PanelProps, type GrafanaTheme2 } from '@grafana/data';
import { useTheme2, useStyles2 } from '@grafana/ui';

import type { Options } from './panelcfg.gen';
import type { MultiMixDataPoint, MultiMixState } from './state';
import { MultiMixStreamHandler } from './streaming';

const ROW_HEIGHT = 20;
const OVERSCAN_COUNT = 20;

interface Props extends PanelProps<Options> {}

export const MultiMixPanel = memo((props: Props) => {
  const { options, data, width, height } = props;
  const theme = useTheme2();
  const styles = useStyles2(getStyles);

  const [state, setState] = useState<MultiMixState>({
    dataPoints: [],
    isLoading: false,
    error: null,
    streamActive: false,
    totalPoints: 0,
  });

  const streamHandlerRef = useRef<MultiMixStreamHandler | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [scrollTop, setScrollTop] = useState(0);
  const [containerHeight, setContainerHeight] = useState(0);

  const visibleRange = useMemo(() => {
    const startIdx = Math.max(0, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN_COUNT);
    const endIdx = Math.min(
      state.dataPoints.length,
      Math.ceil((scrollTop + containerHeight) / ROW_HEIGHT) + OVERSCAN_COUNT
    );
    return { startIdx, endIdx };
  }, [scrollTop, containerHeight, state.dataPoints.length]);

  const visiblePoints = useMemo(() => {
    return state.dataPoints.slice(visibleRange.startIdx, visibleRange.endIdx);
  }, [state.dataPoints, visibleRange]);

  useEffect(() => {
    if (data?.series && data.series.length > 0) {
      const frames = data.series;
      const frame = frames[0];

      if (frame.fields && frame.fields.length >= 2) {
        const timeField = frame.fields.find((f) => f.name === 'time');
        const promField = frame.fields.find((f) => f.name === 'prometheus_value');
        const lokiField = frame.fields.find((f) => f.name === 'loki_log_count');

        if (timeField && timeField.values) {
          const timeValues = timeField.values as number[];
          const points: MultiMixDataPoint[] = [];

          for (let i = 0; i < timeValues.length; i++) {
            points.push({
              time: timeValues[i],
              prometheusValue: (promField?.values?.[i] as number | null) ?? null,
              lokiLogCount: (lokiField?.values?.[i] as number | null) ?? null,
            });
          }

          setState((prev) => ({
            ...prev,
            dataPoints: points,
            totalPoints: points.length,
            isLoading: false,
          }));
        }
      }
      return;
    }

    if (options.enableStreaming && data?.request?.targets) {
      const dsUid = data.request.targets[0]?.datasource?.uid;
      if (dsUid && (options.prometheusQuery || options.lokiQuery)) {
        if (!streamHandlerRef.current) {
          streamHandlerRef.current = new MultiMixStreamHandler();
        }

        const handler = streamHandlerRef.current;

        const sub = handler.dataStream.subscribe((points) => {
          setState((prev) => {
            if (points.length === 0) {
              return prev;
            }
            return {
              ...prev,
              dataPoints: [...prev.dataPoints, ...points],
              totalPoints: prev.dataPoints.length + points.length,
              streamActive: true,
            };
          });
        });

        handler.start({
          datasourceUid: dsUid,
          prometheusQuery: options.prometheusQuery,
          lokiQuery: options.lokiQuery,
          step: options.step,
        });

        return () => {
          sub.unsubscribe();
          handler.stop();
        };
      }
    }

    return () => {
      streamHandlerRef.current?.stop();
    };
  }, [data, options.enableStreaming, options.prometheusQuery, options.lokiQuery, options.step]);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) {
      return;
    }

    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerHeight(entry.contentRect.height);
      }
    });

    observer.observe(container);
    return () => observer.disconnect();
  }, []);

  const handleScroll = useCallback((e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  }, []);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas || !width || !height) {
      return;
    }

    const ctx = canvas.getContext('2d');
    if (!ctx) {
      return;
    }

    const dpr = window.devicePixelRatio || 1;
    canvas.width = width * dpr;
    canvas.height = height * dpr;
    ctx.scale(dpr, dpr);

    ctx.clearRect(0, 0, width, height);

    if (visiblePoints.length === 0) {
      return;
    }

    const chartHeight = height - 40;
    const chartTop = 20;
    const chartWidth = width - 60;
    const chartLeft = 50;

    let promMin = Infinity;
    let promMax = -Infinity;
    let lokiMin = Infinity;
    let lokiMax = -Infinity;

    for (const point of visiblePoints) {
      if (point.prometheusValue != null) {
        promMin = Math.min(promMin, point.prometheusValue);
        promMax = Math.max(promMax, point.prometheusValue);
      }
      if (point.lokiLogCount != null) {
        lokiMin = Math.min(lokiMin, point.lokiLogCount);
        lokiMax = Math.max(lokiMax, point.lokiLogCount);
      }
    }

    if (promMin === Infinity) {
      promMin = 0;
    }
    if (promMax === -Infinity) {
      promMax = 1;
    }
    if (lokiMin === Infinity) {
      lokiMin = 0;
    }
    if (lokiMax === -Infinity) {
      lokiMax = 1;
    }

    const promRange = promMax - promMin || 1;
    const lokiRange = lokiMax - lokiMin || 1;

    ctx.strokeStyle = theme.colors.primary.main;
    ctx.lineWidth = 1.5;
    ctx.beginPath();

    let started = false;
    for (let i = 0; i < visiblePoints.length; i++) {
      const point = visiblePoints[i];
      if (point.prometheusValue == null) {
        continue;
      }

      const x = chartLeft + (i / visiblePoints.length) * chartWidth;
      const y = chartTop + chartHeight - ((point.prometheusValue - promMin) / promRange) * chartHeight;

      if (!started) {
        ctx.moveTo(x, y);
        started = true;
      } else {
        ctx.lineTo(x, y);
      }
    }
    ctx.stroke();

    ctx.strokeStyle = theme.colors.warning.main;
    ctx.lineWidth = 1.5;
    ctx.beginPath();

    started = false;
    for (let i = 0; i < visiblePoints.length; i++) {
      const point = visiblePoints[i];
      if (point.lokiLogCount == null) {
        continue;
      }

      const x = chartLeft + (i / visiblePoints.length) * chartWidth;
      const y = chartTop + chartHeight - ((point.lokiLogCount - lokiMin) / lokiRange) * chartHeight;

      if (!started) {
        ctx.moveTo(x, y);
        started = true;
      } else {
        ctx.lineTo(x, y);
      }
    }
    ctx.stroke();

    ctx.fillStyle = theme.colors.text.primary;
    ctx.font = '11px sans-serif';
    ctx.fillText('Prometheus', chartLeft + 10, 12);
    ctx.fillStyle = theme.colors.warning.main;
    ctx.fillText('Loki', chartLeft + 100, 12);

    ctx.fillStyle = theme.colors.text.secondary;
    ctx.font = '10px sans-serif';
    const promMinLabel = promMin.toFixed(1);
    const promMaxLabel = promMax.toFixed(1);
    ctx.fillText(promMinLabel, 5, chartTop + chartHeight);
    ctx.fillText(promMaxLabel, 5, chartTop + 12);
  }, [visiblePoints, width, height, theme]);

  return (
    <div ref={containerRef} className={styles.container} onScroll={handleScroll} style={{ height }}>
      {state.error && (
        <div className={styles.error}>
          {state.error}
        </div>
      )}
      <canvas ref={canvasRef} className={styles.canvas} style={{ width, height }} />
      {state.dataPoints.length > 0 && (
        <div className={styles.stats}>
          {state.streamActive && <span className={styles.liveIndicator}>LIVE</span>}
          <span>{state.dataPoints.length.toLocaleString()} data points</span>
        </div>
      )}
    </div>
  );
});

MultiMixPanel.displayName = 'MultiMixPanel';

const getStyles = (theme: GrafanaTheme2) => ({
  container: css({
    position: 'relative',
    overflow: 'auto',
    width: '100%',
    height: '100%',
  }),
  canvas: css({
    position: 'absolute',
    top: 0,
    left: 0,
  }),
  error: css({
    position: 'absolute',
    top: 8,
    left: 8,
    right: 8,
    padding: '8px 12px',
    background: theme.colors.error.main,
    color: theme.colors.error.contrastText,
    borderRadius: theme.shape.radius.default,
    zIndex: 10,
    fontSize: 12,
  }),
  stats: css({
    position: 'absolute',
    bottom: 8,
    right: 8,
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    fontSize: 11,
    color: theme.colors.text.secondary,
    background: theme.colors.background.canvas,
    padding: '4px 8px',
    borderRadius: theme.shape.radius.default,
  }),
  liveIndicator: css({
    display: 'inline-block',
    padding: '2px 6px',
    background: theme.colors.success.main,
    color: theme.colors.success.contrastText,
    borderRadius: 3,
    fontSize: 10,
    fontWeight: 600,
  }),
});