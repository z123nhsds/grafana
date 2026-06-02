import { createSlice, type PayloadAction } from '@reduxjs/toolkit';

import type { FieldType, DataFrame } from '@grafana/data';

export interface MultiMixDataPoint {
  time: number;
  prometheusValue: number | null;
  lokiLogCount: number | null;
}

export interface MultiMixState {
  dataPoints: MultiMixDataPoint[];
  isLoading: boolean;
  error: string | null;
  streamActive: boolean;
  totalPoints: number;
}

const initialState: MultiMixState = {
  dataPoints: [],
  isLoading: false,
  error: null,
  streamActive: false,
  totalPoints: 0,
};

const multiMixSlice = createSlice({
  name: 'multimix',
  initialState,
  reducers: {
    setLoading(state, action: PayloadAction<boolean>) {
      state.isLoading = action.payload;
    },
    setError(state, action: PayloadAction<string | null>) {
      state.error = action.payload;
    },
    setStreamActive(state, action: PayloadAction<boolean>) {
      state.streamActive = action.payload;
    },
    appendDataPoints(state, action: PayloadAction<MultiMixDataPoint[]>) {
      state.dataPoints = [...state.dataPoints, ...action.payload];
      state.totalPoints = state.dataPoints.length;
    },
    updateDataPoints(state, action: PayloadAction<MultiMixDataPoint[]>) {
      state.dataPoints = action.payload;
      state.totalPoints = action.payload.length;
    },
    clearData(state) {
      state.dataPoints = [];
      state.totalPoints = 0;
      state.error = null;
    },
    processFrames(state, action: PayloadAction<DataFrame[]>) {
      const frames = action.payload;
      if (!frames || frames.length === 0) {
        return;
      }

      const frame = frames[0];
      if (!frame.fields || frame.fields.length < 2) {
        return;
      }

      const timeField = frame.fields.find((f) => f.type === 'time' as FieldType);
      if (!timeField || !timeField.values) {
        return;
      }

      const points: MultiMixDataPoint[] = [];
      const timeValues = timeField.values as number[];

      for (let i = 0; i < timeValues.length; i++) {
        const point: MultiMixDataPoint = {
          time: timeValues[i],
          prometheusValue: null,
          lokiLogCount: null,
        };

        for (let j = 1; j < frame.fields.length; j++) {
          const field = frame.fields[j];
          if (field.values && field.values[i] != null) {
            if (field.name === 'prometheus_value') {
              point.prometheusValue = field.values[i] as number;
            } else if (field.name === 'loki_log_count') {
              point.lokiLogCount = field.values[i] as number;
            }
          }
        }

        points.push(point);
      }

      state.dataPoints = points;
      state.totalPoints = points.length;
    },
  },
});

export const {
  setLoading,
  setError,
  setStreamActive,
  appendDataPoints,
  updateDataPoints,
  clearData,
  processFrames,
} = multiMixSlice.actions;

export default multiMixSlice.reducer;