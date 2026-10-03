import { RecordHistory } from '../workspace/record-history';
import type { ItemViewProps } from './types';

// The existing comments/timeline feature is reused without copying its forms or API.
export function ActivityView({ section, record, viewer }: ItemViewProps) {
  return <RecordHistory sectionID={section.id} recordID={record.id} viewer={viewer} />;
}
