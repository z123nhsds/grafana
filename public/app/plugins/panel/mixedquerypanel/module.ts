import { PanelPlugin } from '@grafana/data';
import { MixedQueryPanel } from './MixedQueryPanel';
import { MixedQueryOptions, defaults } from './types';

export const plugin = new PanelPlugin<MixedQueryOptions>(MixedQueryPanel).setPanelOptions((builder) => {
  return builder
    .addTextInput({
      path: 'title',
      name: 'Panel title',
      defaultValue: defaults.title,
    })
    .addNumberInput({
      path: 'virtualScrollItemHeight',
      name: 'Item height for virtual scroll',
      defaultValue: defaults.virtualScrollItemHeight,
    });
});
