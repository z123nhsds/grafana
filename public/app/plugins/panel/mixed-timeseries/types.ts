export interface MixedTimeseriesOptions {
  enableStreaming: boolean;
  maxPoints: number;
  showPrometheus: boolean;
  showLoki: boolean;
  alignmentMs: number;
}

export interface MixedTimeseriesFieldConfig {
  prometheusColor: string;
  lokiColor: string;
  lineWidth: number;
}

export interface TimeSeriesPoint {
  time: number;
  prometheus: number;
  loki: number;
}

export interface MixedTimeseriesState {
  streamData: TimeSeriesPoint[];
  isStreaming: boolean;
  error: string | null;
}
