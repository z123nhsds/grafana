import { PanelPlugin } from '@grafana/data';
import { MixedTimeseriesPanel } from './MixedTimeseriesPanel';
import { MixedTimeseriesOptions, MixedTimeseriesFieldConfig } from './types';

const defaultOptions: MixedTimeseriesOptions = {
  enableStreaming: false,
  maxPoints: 1000000,
  showPrometheus: true,
  showLoki: true,
  alignmentMs: 1000,
};

const defaultFieldConfig: MixedTimeseriesFieldConfig = {
  prometheusColor: '#5794F2',
  lokiColor: '#7EB26D',
  lineWidth: 2,
};

export const plugin = new PanelPlugin<MixedTimeseriesOptions, MixedTimeseriesFieldConfig>(MixedTimeseriesPanel)
  .setPanelOptions((builder) => {
    builder
      .addBooleanSwitch({
        path: 'enableStreaming',
        name: 'Enable Streaming',
        description: 'Enable real-time streaming data updates',
        defaultValue: defaultOptions.enableStreaming,
      })
      .addNumberInput({
        path: 'maxPoints',
        name: 'Max Points',
        description: 'Maximum number of data points to render (for virtual scrolling)',
        defaultValue: defaultOptions.maxPoints,
      })
      .addBooleanSwitch({
        path: 'showPrometheus',
        name: 'Show Prometheus',
        description: 'Display Prometheus data series',
        defaultValue: defaultOptions.showPrometheus,
      })
      .addBooleanSwitch({
        path: 'showLoki',
        name: 'Show Loki',
        description: 'Display Loki data series',
        defaultValue: defaultOptions.showLoki,
      })
      .addNumberInput({
        path: 'alignmentMs',
        name: 'Alignment (ms)',
        description: 'Time alignment granularity in milliseconds',
        defaultValue: defaultOptions.alignmentMs,
      });
  })
  .useFieldConfig({
    standardOptions: {},
    useCustomConfig: (builder) => {
      builder
        .addColorPicker({
          path: 'prometheusColor',
          name: 'Prometheus Color',
          defaultValue: defaultFieldConfig.prometheusColor,
        })
        .addColorPicker({
          path: 'lokiColor',
          name: 'Loki Color',
          defaultValue: defaultFieldConfig.lokiColor,
        })
        .addNumberInput({
          path: 'lineWidth',
          name: 'Line Width',
          defaultValue: defaultFieldConfig.lineWidth,
        });
    },
  });
