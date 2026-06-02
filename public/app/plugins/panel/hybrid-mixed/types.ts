export interface HybridMixedPanelOptions {
  datasourceUid?: string;
  liveUpdates: boolean;
  rowHeight: number;
  batchSize: number;
  pushEveryMs: number;
}

export const defaultPanelOptions: HybridMixedPanelOptions = {
  datasourceUid: '',
  liveUpdates: true,
  rowHeight: 28,
  batchSize: 256,
  pushEveryMs: 25,
};

export interface HybridColumns {
  timestamps: number[];
  prometheus: Array<number | null>;
  loki: Array<number | null>;
  merged: Array<number | null>;
}
