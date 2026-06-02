export interface Options {
  prometheusQuery: string;
  lokiQuery: string;
  step: string;
  mergeMode: MergeMode;
  enableStreaming: boolean;
  maxDataPoints: number;
  showPrometheus: boolean;
  showLoki: boolean;
}

export enum MergeMode {
  Inner = 'inner',
  Left = 'left',
  Outer = 'outer',
}

export const defaultOptions: Partial<Options> = {
  prometheusQuery: '',
  lokiQuery: '',
  step: '15s',
  mergeMode: MergeMode.Inner,
  enableStreaming: true,
  maxDataPoints: 1000000,
  showPrometheus: true,
  showLoki: true,
};