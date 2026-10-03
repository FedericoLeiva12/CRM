import type { ItemViewDefinition, SectionView } from '../../types/crm';
import { ActivityView } from './activity-view';
import { InfoView } from './info-view';
import type { ItemViewPlugin } from './types';

// Frontend extension point: register a renderer under the backend catalog's ID.
// Components own their UI and data needs; the shared screen owns navigation.
export const itemViewPlugins: Record<string, ItemViewPlugin> = {
  info: { Component: InfoView },
  activity: { Component: ActivityView },
};

export function enabledItemViews(views: SectionView[], catalog: ItemViewDefinition[]) {
  const info: SectionView = { id: 'info', enabled: true, config: {} };
  const optional = views.filter(
    (view) => view.id !== 'info' && view.enabled && itemViewPlugins[view.id],
  );
  return [info, ...optional].map((settings) => ({
    settings,
    definition: catalog.find((definition) => definition.id === settings.id),
    plugin: itemViewPlugins[settings.id],
  }));
}
