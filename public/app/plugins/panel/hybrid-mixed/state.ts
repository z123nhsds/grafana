import { configureStore, createSlice, type PayloadAction } from '@reduxjs/toolkit';

import { type HybridColumns } from './types';

export type StreamStatus = 'idle' | 'streaming' | 'error';

export interface HybridMixedPanelState {
  timestamps: number[];
  prometheus: Array<number | null>;
  loki: Array<number | null>;
  merged: Array<number | null>;
  streamStatus: StreamStatus;
  error?: string;
}

const initialState: HybridMixedPanelState = {
  timestamps: [],
  prometheus: [],
  loki: [],
  merged: [],
  streamStatus: 'idle',
};

const panelSlice = createSlice({
  name: 'hybridMixedPanel',
  initialState,
  reducers: {
    replaceColumns: (state, action: PayloadAction<HybridColumns>) => {
      state.timestamps = action.payload.timestamps;
      state.prometheus = action.payload.prometheus;
      state.loki = action.payload.loki;
      state.merged = action.payload.merged;
      state.error = undefined;
    },
    appendColumns: (state, action: PayloadAction<HybridColumns>) => {
      state.timestamps.push(...action.payload.timestamps);
      state.prometheus.push(...action.payload.prometheus);
      state.loki.push(...action.payload.loki);
      state.merged.push(...action.payload.merged);
    },
    setStreamStatus: (state, action: PayloadAction<StreamStatus>) => {
      state.streamStatus = action.payload;
      if (action.payload !== 'error') {
        state.error = undefined;
      }
    },
    setError: (state, action: PayloadAction<string>) => {
      state.streamStatus = 'error';
      state.error = action.payload;
    },
  },
});

export const { appendColumns, replaceColumns, setError, setStreamStatus } = panelSlice.actions;

export function createHybridMixedPanelStore() {
  return configureStore({
    reducer: {
      panel: panelSlice.reducer,
    },
  });
}

export type HybridMixedPanelStore = ReturnType<typeof createHybridMixedPanelStore>;
export type HybridMixedPanelRootState = ReturnType<HybridMixedPanelStore['getState']>;
