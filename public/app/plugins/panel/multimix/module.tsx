import { PanelPlugin } from '@grafana/data';
import { t } from '@grafana/i18n';

import { MultiMixPanel } from './MultiMixPanel';
import { defaultOptions, MergeMode, type Options } from './panelcfg.gen';

export const plugin = new PanelPlugin<Options>(MultiMixPanel)
  .setPanelOptions((builder) => {
    const category = [t('multimix.category-queries', 'Queries')];

    builder
      .addTextInput({
        path: 'prometheusQuery',
        name: t('multimix.name-prometheus-query', 'Prometheus Query'),
        description: t('multimix.description-prometheus-query', 'PromQL query for metrics data'),
        category,
        defaultValue: defaultOptions.prometheusQuery,
      })
      .addTextInput({
        path: 'lokiQuery',
        name: t('multimix.name-loki-query', 'Loki Query'),
        description: t('multimix.description-loki-query', 'LogQL query for log data'),
        category,
        defaultValue: defaultOptions.lokiQuery,
      })
      .addTextInput({
        path: 'step',
        name: t('multimix.name-step', 'Step'),
        description: t('multimix.description-step', 'Time step for query resolution (e.g. 15s, 1m)'),
        category,
        defaultValue: defaultOptions.step,
      })
      .addSelect({
        path: 'mergeMode',
        name: t('multimix.name-merge-mode', 'Merge Mode'),
        description: t('multimix.description-merge-mode', 'How to align Prometheus and Loki time series'),
        category,
        settings: {
          options: [
            { value: MergeMode.Inner, label: t('multimix.merge-mode.inner', 'Inner Join') },
            { value: MergeMode.Left, label: t('multimix.merge-mode.left', 'Left Join') },
            { value: MergeMode.Outer, label: t('multimix.merge-mode.outer', 'Outer Join') },
          ],
        },
        defaultValue: defaultOptions.mergeMode,
      })
      .addBooleanSwitch({
        path: 'enableStreaming',
        name: t('multimix.name-enable-streaming', 'Enable Streaming'),
        description: t('multimix.description-enable-streaming', 'Stream live data updates from the backend'),
        category,
        defaultValue: defaultOptions.enableStreaming,
      })
      .addBooleanSwitch({
        path: 'showPrometheus',
        name: t('multimix.name-show-prometheus', 'Show Prometheus'),
        category,
        defaultValue: defaultOptions.showPrometheus,
      })
      .addBooleanSwitch({
        path: 'showLoki',
        name: t('multimix.name-show-loki', 'Show Loki'),
        category,
        defaultValue: defaultOptions.showLoki,
      });
  })
  .setSuggestionsSupplier(() => []);