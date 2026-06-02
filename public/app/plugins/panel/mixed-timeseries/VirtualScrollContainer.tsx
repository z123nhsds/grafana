import React, { useMemo, useRef, useCallback, useState, useEffect } from 'react';
import { css } from '@emotion/css';
import { TimeSeriesPoint, MixedTimeseriesOptions } from './types';

interface VirtualScrollContainerProps {
  data: TimeSeriesPoint[];
  width: number;
  height: number;
  rowHeight: number;
  options: MixedTimeseriesOptions;
}

const OVERSCAN_COUNT = 5;

export const VirtualScrollContainer: React.FC<VirtualScrollContainerProps> = ({
  data,
  width,
  height,
  rowHeight,
  options,
}) => {
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);

  const totalHeight = useMemo(() => data.length * rowHeight, [data.length, rowHeight]);
  const visibleRowCount = useMemo(() => Math.ceil(height / rowHeight), [height, rowHeight]);

  const startIndex = useMemo(() => {
    const index = Math.floor(scrollTop / rowHeight);
    return Math.max(0, index - OVERSCAN_COUNT);
  }, [scrollTop, rowHeight]);

  const endIndex = useMemo(() => {
    const index = startIndex + visibleRowCount + OVERSCAN_COUNT * 2;
    return Math.min(data.length, index);
  }, [startIndex, visibleRowCount, data.length]);

  const visibleData = useMemo(() => {
    return data.slice(startIndex, endIndex);
  }, [data, startIndex, endIndex]);

  const handleScroll = useCallback(() => {
    if (containerRef.current) {
      setScrollTop(containerRef.current.scrollTop);
    }
  }, []);

  useEffect(() => {
    const container = containerRef.current;
    if (!container) return;

    container.addEventListener('scroll', handleScroll, { passive: true });
    return () => container.removeEventListener('scroll', handleScroll);
  }, [handleScroll]);

  const offsetY = startIndex * rowHeight;

  return (
    <div
      ref={containerRef}
      style={{
        width,
        height,
        overflow: 'auto',
        position: 'relative',
      }}
    >
      <div
        style={{
          height: totalHeight,
          position: 'relative',
          overflow: 'hidden',
        }}
      >
        <div
          style={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            transform: `translateY(${offsetY}px)`,
          }}
        >
          {visibleData.map((point, index) => (
            <div
              key={startIndex + index}
              style={{
                height: rowHeight,
                display: 'flex',
                alignItems: 'center',
                padding: '0 8px',
                borderBottom: '1px solid #e0e0e0',
              }}
            >
              <div style={{ width: 120, fontSize: 12, color: '#666' }}>
                {new Date(point.time).toLocaleTimeString()}
              </div>
              {options.showPrometheus && (
                <div
                  style={{
                    flex: 1,
                    height: 20,
                    backgroundColor: options.prometheusColor || '#5794F2',
                    transform: `scaleX(${Math.min(1, Math.max(0, point.prometheus / 100))})`,
                    transformOrigin: 'left',
                    borderRadius: 2,
                  }}
                />
              )}
              {options.showLoki && (
                <div
                  style={{
                    flex: 1,
                    height: 20,
                    backgroundColor: options.lokiColor || '#7EB26D',
                    transform: `scaleX(${Math.min(1, Math.max(0, point.loki / 100))})`,
                    transformOrigin: 'left',
                    borderRadius: 2,
                  }}
                />
              )}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
};
