import { Subject, Observable, timer, Subscription } from 'rxjs';
import { takeUntil, switchMap, retry, catchError } from 'rxjs/operators';

import { getBackendSrv, type BackendSrv } from '@grafana/runtime';
import type { DataFrame, DataQueryResponse } from '@grafana/data';

import type { MultiMixDataPoint } from './state';

export interface StreamConfig {
  datasourceUid: string;
  prometheusQuery: string;
  lokiQuery: string;
  step: string;
}

export class MultiMixStreamHandler {
  private stopSubject = new Subject<void>();
  private dataSubject = new Subject<MultiMixDataPoint[]>();
  private subscription: Subscription | null = null;
  private backendSrv: BackendSrv;

  constructor() {
    this.backendSrv = getBackendSrv();
  }

  get dataStream(): Observable<MultiMixDataPoint[]> {
    return this.dataSubject.asObservable();
  }

  start(config: StreamConfig): void {
    this.stop();

    this.subscription = timer(0, 1000)
      .pipe(
        takeUntil(this.stopSubject),
        switchMap(() => {
          return new Observable<MultiMixDataPoint[]>((observer) => {
            this.fetchStreamData(config)
              .then((points) => {
                observer.next(points);
                observer.complete();
              })
              .catch((err) => {
                observer.error(err);
              });
          });
        }),
        retry({ count: 3, delay: 2000 }),
        catchError((err) => {
          console.error('MultiMix stream error:', err);
          return [];
        })
      )
      .subscribe({
        next: (points) => {
          this.dataSubject.next(points);
        },
        error: (err) => {
          console.error('MultiMix stream subscription error:', err);
        },
      });
  }

  private async fetchStreamData(config: StreamConfig): Promise<MultiMixDataPoint[]> {
    try {
      const response = await this.backendSrv.post<DataQueryResponse>(
        `/api/datasources/uid/${config.datasourceUid}/resources/query`,
        {
          prometheusQuery: config.prometheusQuery,
          lokiQuery: config.lokiQuery,
          step: config.step,
        }
      );

      return this.extractPoints(response.data ?? []);
    } catch {
      return [];
    }
  }

  private extractPoints(frames: DataFrame[]): MultiMixDataPoint[] {
    if (!frames || frames.length === 0) {
      return [];
    }

    const frame = frames[0];
    if (!frame.fields || frame.fields.length < 2) {
      return [];
    }

    const timeField = frame.fields.find((f) => f.name === 'time');
    if (!timeField || !timeField.values) {
      return [];
    }

    const promField = frame.fields.find((f) => f.name === 'prometheus_value');
    const lokiField = frame.fields.find((f) => f.name === 'loki_log_count');

    const points: MultiMixDataPoint[] = [];
    const timeValues = timeField.values as number[];

    for (let i = 0; i < timeValues.length; i++) {
      points.push({
        time: timeValues[i],
        prometheusValue: promField?.values?.[i] ?? null,
        lokiLogCount: lokiField?.values?.[i] ?? null,
      });
    }

    return points;
  }

  stop(): void {
    this.stopSubject.next();
    if (this.subscription) {
      this.subscription.unsubscribe();
      this.subscription = null;
    }
  }

  destroy(): void {
    this.stop();
    this.stopSubject.complete();
    this.dataSubject.complete();
  }
}