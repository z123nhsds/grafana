import React, { useEffect, useState, useRef, useCallback, useMemo } from 'react';
import { PanelProps } from '@grafana/data';
import { useAppDispatch, useAppSelector } from 'app/store/store';
import { Subject, from, merge, interval } from 'rxjs';
import { switchMap, takeUntil, filter, bufferTime, map } from 'rxjs/operators';
import { css, cx } from '@emotion/css';
import { MixedTimeseriesOptions, MixedTimeseriesFieldConfig } from './types';
import { VirtualScrollContainer } from './VirtualScrollContainer';
import { StreamingDataProcessor } from './StreamingDataProcessor';
import { setDataStream, addDataPoint, clearDataStream } from './slice';

interface Props extends PanelProps<MixedTimeseriesOptions, MixedTimeseriesFieldConfig> {}

const ROW_HEIGHT = 30;
const BUFFER_INTERVAL_MS = 100;

export const MixedTimeseriesPanel: React.FC<Props> = ({
  data,
  width,
  height,
  options,
  fieldConfig,
  replaceVariables,
}) => {
  const dispatch = useAppDispatch();
  const streamData = useAppSelector((state) => state.mixedTimeseries.streamData);
  const [localPoints, setLocalPoints] = useState<Array<{ time: number; prometheus: number; loki: number }>>([]);
  const stopSignalRef = useRef<Subject<void>>(new Subject<void>());
  const containerRef = useRef<HTMLDivElement>(null);

  const dataStream$ = useMemo(() => new Subject<Array<{ time: number; prometheus: number; loki: number }>>(), []);

  useEffect(() => {
    stopSignalRef.current = new Subject<void>();

    const subscription = merge(
      dataStream$.pipe(
        bufferTime(BUFFER_INTERVAL_MS),
        filter((batch) => batch.length > 0),
        map((batch) => batch.flat())
      )
    )
    .pipe(
      takeUntil(stopSignalRef.current)
    )
    .subscribe((points) => {
      setLocalPoints((prev) => {
        const merged = [...prev, ...points];
        const maxPoints = options.maxPoints || 1000000;
        return merged.slice(-maxPoints);
      });
    });

    return () => {
      stopSignalRef.current.next();
      stopSignalRef.current.complete();
      subscription.unsubscribe();
    };
  }, [dataStream$, options.maxPoints]);

  useEffect(() => {
    if (data.series?.length > 0) {
      const frames = data.series;
      const points: Array<{ time: number; prometheus: number; loki: number }> = [];

      for (const frame of frames) {
        if (frame.fields?.length >= 3) {
          const timeField = frame.fields[0];
          const promField = frame.fields[1];
          const lokiField = frame.fields[2];

          for (let i = 0; i < frame.length; i++) {
            const time = timeField.values.get(i);
            const prom = promField.values.get(i);
            const loki = lokiField.values.get(i);

            if (time != null && prom != null && loki != null) {
              points.push({
                time: new Date(time).getTime(),
                prometheus: prom,
                loki: loki,
              });
            }
          }
        }
      }

      setLocalPoints(points);
      dataStream$.next(points);
    }
  }, [data.series, dataStream$]);

  const handleStreamingUpdate = useCallback((newPoints: Array<{ time: number; prometheus: number; loki: number }>) => {
    dataStream$.next(newPoints);
    dispatch(addDataPoint(newPoints));
  }, [dataStream$, dispatch]);

  const sortedPoints = useMemo(() => {
    return [...localPoints].sort((a, b) => a.time - b.time);
  }, [localPoints]);

  const visibleWidth = width - 40;

  return (
    <div
      ref={containerRef}
      className={cx(
        css`
          width: 100%;
          height: 100%;
          overflow: hidden;
          position: relative;
        `
      )}
    >
      <VirtualScrollContainer
        data={sortedPoints}
        width={visibleWidth}
        height={height}
        rowHeight={ROW_HEIGHT}
        options={options}
      />
      <StreamingDataProcessor
        onStreamingUpdate={handleStreamingUpdate}
        enabled={options.enableStreaming}
      />
    </div>
  );
};
