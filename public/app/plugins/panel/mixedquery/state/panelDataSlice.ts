import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import { type PanelData, type DataFrame, LoadingState } from '@grafana/data';

interface PanelDataState {
  series: DataFrame[];
  loadingState: LoadingState;
  error?: Error;
  timeRange: PanelData['timeRange'];
  receivedAt: number;
}

export const initialState: PanelDataState = {
  series: [],
  loadingState: LoadingState.NotStarted,
  timeRange: { from: new Date(), to: new Date(), raw: { from: 'now-1h', to: 'now' } },
  receivedAt: 0,
};

const panelDataSlice = createSlice({
  name: 'panelData',
  initialState,
  reducers: {
    receiveData(state, action: PayloadAction<PanelData>) {
      state.series = action.payload.series;
      state.loadingState = action.payload.state;
      state.timeRange = action.payload.timeRange;
      state.receivedAt = Date.now();
    },
    appendData(state, action: PayloadAction<PanelData>) {
      const existingNames = new Set(state.series.map((s) => s.name));
      const newSeries = action.payload.series.filter((s) => !existingNames.has(s.name));
      state.series = [...state.series, ...newSeries];
      state.loadingState = action.payload.state;
      state.receivedAt = Date.now();
    },
    complete(state, action: PayloadAction<PanelData>) {
      state.loadingState = LoadingState.Done;
      state.timeRange = action.payload.timeRange;
      state.receivedAt = Date.now();
    },
    error(state, action: PayloadAction<{ error: Error }>) {
      state.error = action.payload.error;
      state.loadingState = LoadingState.Error;
    },
    reset(state) {
      state.series = [];
      state.loadingState = LoadingState.NotStarted;
      state.error = undefined;
    },
  },
});

export const { receiveData, appendData, complete, error, reset } = panelDataSlice.actions;
export const panelDataReducer = panelDataSlice.reducer;
export default panelDataSlice;
