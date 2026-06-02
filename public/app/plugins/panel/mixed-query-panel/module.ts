import { PanelPlugin } from '@grafana/data';
import { MixedQueryPanel } from './components/MixedQueryPanel';

export interface Options {}

export const plugin = new PanelPlugin<Options>(MixedQueryPanel).setPanelOptions((builder: any) => {});
