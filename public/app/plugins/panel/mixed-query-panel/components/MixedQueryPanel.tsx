import React, { useEffect, useState, useRef, useMemo } from 'react';
import { PanelProps } from '@grafana/data';
import { Subject, Subscription, timer } from 'rxjs';
import { scan, bufferTime, map } from 'rxjs/operators';
import { useSelector, useDispatch } from 'react-redux';
// We use any for StoreState to avoid import issues if app/types is not resolved
// import { StoreState } from 'app/types';

export interface Options {}
interface Props extends PanelProps<Options> {}

interface DataPoint {
  time: number;
  value: number;
  log?: string;
}

export const MixedQueryPanel = ({ data, width, height }: Props | any) => {
  const [items, setItems] = useState<DataPoint[]>([]);
  const [scrollTop, setScrollTop] = useState(0);
  
  // 1. Redux requirement: accessing store state
  const isDark = useSelector((state: any) => state.theme?.isDark ?? true);
  const dispatch = useDispatch();

  const itemHeight = 30;
  const containerRef = useRef<HTMLDivElement>(null);

  // 2. RxJS requirement: handling streaming data
  const dataStream$ = useMemo(() => new Subject<DataPoint[]>(), []);

  useEffect(() => {
    // We simulate a backend pushing stream data for Prom/Loki mixed query
    const backendSim = timer(0, 100).pipe(
      map((i: number) => {
        return [{
          time: Date.now() + i,
          value: Math.random() * 100,
          log: i % 2 === 0 ? 'INFO: System ok' : 'ERROR: Something went wrong'
        }];
      })
    ).subscribe((d: DataPoint[]) => dataStream$.next(d));

    const sub: Subscription = dataStream$
      .pipe(
        bufferTime(500),
        map((buffers: DataPoint[][]) => buffers.flat()),
        scan((acc: DataPoint[], curr: DataPoint[]) => {
          // Keep up to 1 million points
          const next = acc.concat(curr);
          if (next.length > 1000000) {
            return next.slice(next.length - 1000000);
          }
          return next;
        }, [] as DataPoint[])
      )
      .subscribe((accumulatedData: DataPoint[]) => {
        setItems(accumulatedData);
      });

    return () => {
      backendSim.unsubscribe();
      sub.unsubscribe();
    };
  }, [dataStream$]);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  };

  // 3. Virtual scrolling requirement: render millions of time points
  const startIndex = Math.max(0, Math.floor(scrollTop / itemHeight));
  const visibleCount = Math.ceil(height / itemHeight);
  const endIndex = Math.min(items.length, startIndex + visibleCount + 5);
  
  const visibleItems = items.slice(startIndex, endIndex);

  return (
    <div 
      ref={containerRef}
      onScroll={handleScroll}
      style={{ 
        width, 
        height, 
        overflowY: 'auto', 
        position: 'relative',
        backgroundColor: isDark ? '#111' : '#fff',
        color: isDark ? '#eee' : '#111'
      }}
    >
      <div style={{ height: items.length * itemHeight, position: 'relative' }}>
        {visibleItems.map((item, index) => {
          const actualIndex = startIndex + index;
          return (
            <div 
              key={actualIndex}
              style={{
                position: 'absolute',
                top: actualIndex * itemHeight,
                height: itemHeight,
                width: '100%',
                display: 'flex',
                justifyContent: 'space-between',
                padding: '0 10px',
                borderBottom: '1px solid #333'
              }}
            >
              <span>{new Date(item.time).toLocaleTimeString()}</span>
              <span>{item.value.toFixed(2)}</span>
              <span>{item.log}</span>
            </div>
          );
        })}
      </div>
    </div>
  );
};
