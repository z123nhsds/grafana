import { useEffect, useRef, useCallback } from 'react';
import { Subject, interval, fromEvent } from 'rxjs';
import { switchMap, takeUntil, filter, map, bufferTime } from 'rxjs/operators';

interface StreamingDataProcessorProps {
  onStreamingUpdate: (points: Array<{ time: number; prometheus: number; loki: number }>) => void;
  enabled: boolean;
}

export const StreamingDataProcessor: React.FC<StreamingDataProcessorProps> = ({
  onStreamingUpdate,
  enabled,
}) => {
  const stopSignalRef = useRef<Subject<void>>(new Subject<void>());
  const dataSubjectRef = useRef<Subject<Array<{ time: number; prometheus: number; loki: number }>>>(new Subject());

  const processStream = useCallback(() => {
    if (!enabled) return;

    stopSignalRef.current = new Subject<void>();
    dataSubjectRef.current = new Subject();

    const stream$ = interval(1000).pipe(
      takeUntil(stopSignalRef.current),
      map(() => {
        const now = Date.now();
        return {
          time: now,
          prometheus: Math.random() * 100,
          loki: Math.random() * 100,
        };
      }),
      bufferTime(500),
      filter((batch) => batch.length > 0)
    );

    stream$.subscribe((points) => {
      onStreamingUpdate(points);
    });
  }, [enabled, onStreamingUpdate]);

  useEffect(() => {
    if (enabled) {
      processStream();
    } else {
      stopSignalRef.current.next();
      stopSignalRef.current.complete();
    }

    return () => {
      stopSignalRef.current.next();
      stopSignalRef.current.complete();
    };
  }, [enabled, processStream]);

  return null;
};
