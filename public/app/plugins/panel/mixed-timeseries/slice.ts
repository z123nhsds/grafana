import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import { MixedTimeseriesState, TimeSeriesPoint } from './types';

const initialState: MixedTimeseriesState = {
  streamData: [],
  isStreaming: false,
  error: null,
};

const mixedTimeseriesSlice = createSlice({
  name: 'mixedTimeseries',
  initialState,
  reducers: {
    setDataStream: (state, action: PayloadAction<TimeSeriesPoint[]>) => {
      state.streamData = action.payload;
    },
    addDataPoint: (state, action: PayloadAction<TimeSeriesPoint[]>) => {
      state.streamData = [...state.streamData, ...action.payload];
    },
    clearDataStream: (state) => {
      state.streamData = [];
      state.error = null;
    },
    setStreamingState: (state, action: PayloadAction<boolean>) => {
      state.isStreaming = action.payload;
    },
    setError: (state, action: PayloadAction<string>) => {
      state.error = action.payload;
    },
  },
});

export const { setDataStream, addDataPoint, clearDataStream, setStreamingState, setError } = mixedTimeseriesSlice.actions;
export default mixedTimeseriesSlice.reducer;
