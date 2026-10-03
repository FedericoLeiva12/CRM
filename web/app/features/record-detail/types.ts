import type { ComponentType } from 'react';
import type { RecordDetail, Section, SectionView, Viewer } from '../../types/crm';

// Views receive the record and their own section-level configuration. They never
// decide which views are enabled or whether Info exists; the host owns that policy.
export interface ItemViewProps {
  section: Section;
  record: RecordDetail;
  viewer: Viewer;
  settings: SectionView;
  onPendingChangesChange: (pending: boolean) => void;
}
export interface ItemViewPlugin {
  Component: ComponentType<ItemViewProps>;
}
