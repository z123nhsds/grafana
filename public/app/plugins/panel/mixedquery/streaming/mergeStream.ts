import { Observable, merge, combineLatest, of } from 'rxjs';
import { map, scan, debounceTime, catchError, filter } from 'rxjs/operators';

import { type PanelData, type DataFrame, LoadingState } from '@grafana/data';

export function mergeStreamedData(
  ...sources: Observable<PanelData>[]
): Observable<PanelData> {
  if (sources.length === 0) {
    return of({
      state: LoadingState.Done,
      series: [],
      timeRange: { from: new Date(), to: new Date(), raw: { from: 'now-1h', to: 'now' } },
    });
  }

  return merge(...sources).pipe(
    scan(
      (acc: PanelData, current: PanelData) => {
        const existingKeys = new Set(acc.series.map((s) => s.name));
        const newSeries = current.series.filter((s) => !existingKeys.has(s.name));

        return {
          ...current,
          series: [...acc.series, ...newSeries],
          state: current.state === LoadingState.Done && acc.state === LoadingState.Done
            ? LoadingState.Done
            : LoadingState.Streaming,
        };
      },
      {
        state: LoadingState.Loading,
        series: [],
        timeRange: { from: new Date(), to: new Date(), raw: { from: 'now-1h', to: 'now' } },
      } as PanelData
    ),
    debounceTime(50),
    catchError((error, caught) => {
      return caught;
    })
  );
}

export function createMergeSubject(): {
  subject: any;
  observable: Observable<PanelData>;
} {
  const { Subject } = require('rxjs');
  const subject = new Subject<PanelData>();

  const observable = subject.pipe(
    scan(
      (acc: PanelData, current: PanelData) => {
        const merged = [...acc.series];
        const existingNames = new Set(merged.map((s) => s.name));

        for (const frame of current.series) {
          if (!existingNames.has(frame.name)) {
            merged.push(frame);
            existingNames.add(frame.name);
          }
        }

        return {
          ...current,
          series: merged,
        };
      },
      {
        state: LoadingState.Loading,
        series: [],
        timeRange: { from: new Date(), to: new Date(), raw: { from: 'now-1h', to: 'now' } },
      } as PanelData
    ),
    filter((data) => data.series.length > 0 || data.state !== LoadingState.Loading)
  );

  return { subject, observable };
}

export function alignTimeSeries(frames: DataFrame[]): DataFrame[] {
  if (frames.length === 0) return [];

  const allTimePoints = new Set<number>();

  for (const frame of frames) {
    const timeField = frame.fields.find((f) => f.type === 'time');
    if (timeField) {
      for (const t of timeField.values) {
        allTimePoints.add(t);
      }
    }
  }

  const sortedTimes = Array.from(allTimePoints).sort((a, b) => a - b);

  return frames.map((frame) => {
    const timeField = frame.fields.find((f) => f.type === 'time');
    if (!timeField) return frame;

    const timeIndex = new Map<number, number>();
    timeField.values.forEach((t, i) => timeIndex.set(t, i));

    const newFields = frame.fields.map((field) => {
      if (field.type === 'time') {
        return { ...field, values: sortedTimes };
      }
      if (field.type === 'number') {
        const newValues = sortedTimes.map((t) => {
          const idx = timeIndex.get(t);
          return idx !== undefined ? field.values[idx] : null;
        });
        return { ...field, values: newValues };
      }
      return field;
    });

    return { ...frame, fields: newFields, length: sortedTimes.length };
  });
}
