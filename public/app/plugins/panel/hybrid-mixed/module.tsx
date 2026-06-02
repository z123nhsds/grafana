import { type PanelOptionsEditorBuilder, PanelPlugin } from '@grafana/data';

import { HybridMixedPanel } from './HybridMixedPanel';
import { defaultPanelOptions, type HybridMixedPanelOptions } from './types';

function configureOptions(builder: PanelOptionsEditorBuilder<HybridMixedPanelOptions>) {
  builder
    .addTextInput({
      path: 'datasourceUid',
      name: 'Streaming datasource UID',
      description: 'Overrides the datasource UID used for the streaming resource endpoint.',
      defaultValue: defaultPanelOptions.datasourceUid,
    })
    .addBooleanSwitch({
      path: 'liveUpdates',
      name: 'Enable streaming',
      defaultValue: defaultPanelOptions.liveUpdates,
    })
    .addNumberInput({
      path: 'rowHeight',
      name: 'Row height',
      defaultValue: defaultPanelOptions.rowHeight,
      settings: {
        integer: true,
        min: 20,
        max: 64,
      },
    })
    .addNumberInput({
      path: 'batchSize',
      name: 'Streaming batch size',
      defaultValue: defaultPanelOptions.batchSize,
      settings: {
        integer: true,
        min: 32,
        max: 2048,
      },
    })
    .addNumberInput({
      path: 'pushEveryMs',
      name: 'Push cadence (ms)',
      defaultValue: defaultPanelOptions.pushEveryMs,
      settings: {
        integer: true,
        min: 0,
        max: 1000,
      },
    });
}

export const plugin = new PanelPlugin<HybridMixedPanelOptions>(HybridMixedPanel).setPanelOptions(configureOptions).setNoPadding();
