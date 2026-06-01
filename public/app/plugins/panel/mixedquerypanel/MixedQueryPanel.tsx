import React, { useEffect, useRef, useMemo, useState } from 'react';
import { PanelProps } from '@grafana/data';
import { MixedQueryOptions } from './types';
import { useStyles2 } from '@grafana/ui';
import { css } from '@emotion/css';

interface Props extends PanelProps<MixedQueryOptions> {}

interface DataItem {
  time: string;
  prometheusValue?: number;
  lokiMessage?: string;
}

export const MixedQueryPanel: React.FC<Props> = ({ options, data, width, height }) => {
  const styles = useStyles2(getStyles);
  const containerRef = useRef<HTMLDivElement>(null);
  const [scrollTop, setScrollTop] = useState(0);

  const allItems = useMemo<DataItem[]>(() => {
    const items: DataItem[] = [];
    for (const frame of data.series) {
      const timeField = frame.fields.find((f) => f.name === 'Time');
      const promField = frame.fields.find((f) => f.name === 'Prometheus_Value');
      const lokiField = frame.fields.find((f) => f.name === 'Loki_Message');

      if (timeField) {
        for (let i = 0; i < timeField.values.length; i++) {
          items.push({
            time: new Date(timeField.values[i]).toLocaleString(),
            prometheusValue: promField?.values[i],
            lokiMessage: lokiField?.values[i],
          });
        }
      }
    }
    return items;
  }, [data.series]);

  const { visibleItems, startIndex } = useMemo(() => {
    const itemHeight = options.virtualScrollItemHeight || 30;
    const containerHeight = height;
    const visibleCount = Math.ceil(containerHeight / itemHeight) + 2;
    const start = Math.floor(scrollTop / itemHeight);
    const end = Math.min(start + visibleCount, allItems.length);

    return {
      visibleItems: allItems.slice(start, end),
      startIndex: start,
    };
  }, [allItems, scrollTop, height, options.virtualScrollItemHeight]);

  const handleScroll = (e: React.UIEvent<HTMLDivElement>) => {
    setScrollTop(e.currentTarget.scrollTop);
  };

  return (
    <div className={styles.container} style={{ width, height }} ref={containerRef} onScroll={handleScroll}>
      <div className={styles.header}>{options.title}</div>
      <div
        className={styles.scrollContainer}
        style={{
          height: height - 50,
        }}
      >
        <div
          style={{
            height: allItems.length * (options.virtualScrollItemHeight || 30),
            position: 'relative',
          }}
        >
          <div
            style={{
              position: 'absolute',
              top: startIndex * (options.virtualScrollItemHeight || 30),
              width: '100%',
            }}
          >
            {visibleItems.map((item, index) => (
              <div
                key={startIndex + index}
                className={styles.item}
                style={{
                  height: options.virtualScrollItemHeight || 30,
                  lineHeight: `${options.virtualScrollItemHeight || 30}px`,
                }}
              >
                <span className={styles.time}>{item.time}</span>
                {item.prometheusValue !== undefined && (
                  <span className={styles.promValue}>Prom: {item.prometheusValue.toFixed(2)}</span>
                )}
                {item.lokiMessage && <span className={styles.lokiMsg}>Loki: {item.lokiMessage}</span>}
              </div>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
};

const getStyles = () => {
  return {
    container: css`
      display: flex;
      flex-direction: column;
      padding: 10px;
      box-sizing: border-box;
    `,
    header: css`
      font-weight: bold;
      margin-bottom: 10px;
      font-size: 16px;
    `,
    scrollContainer: css`
      overflow-y: auto;
      flex: 1;
      border: 1px solid #ddd;
      border-radius: 4px;
    `,
    item: css`
      padding: 0 10px;
      border-bottom: 1px solid #eee;
      display: flex;
      gap: 20px;
    `,
    time: css`
      color: #666;
      min-width: 200px;
    `,
    promValue: css`
      color: #3274d6;
    `,
    lokiMsg: css`
      color: #37872d;
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    `,
  };
};
