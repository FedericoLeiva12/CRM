import * as Tabs from '@radix-ui/react-tabs';
import { Plus } from 'lucide-react';
import { ItemViewsEditor } from './item-views-editor';
import type { ItemViewDefinition, Section } from '../../types/crm';
interface Props {
  catalog: ItemViewDefinition[];
  sections: Section[];
  section: Section;
  onSelectSection: (id: string) => void;
  onAddField: () => void;
}
export function SectionSettings({
  catalog,
  sections,
  section,
  onSelectSection,
  onAddField,
}: Props) {
  const fields = section.fields;
  return (
    <div className="settings-content">
      <div className="section-picker">
        {sections.map((sectionOption) => (
          <button
            key={sectionOption.id}
            className={sectionOption.id === section?.id ? 'selected' : ''}
            onClick={() => onSelectSection(sectionOption.id)}
          >
            {sectionOption.name}
            <span>{sectionOption.fields.length} fields</span>
          </button>
        ))}
      </div>
      <Tabs.Root key={section.id} defaultValue="fields" className="section-settings-panels">
        <Tabs.List aria-label="Section configuration" className="item-tabs">
          <Tabs.Trigger value="fields">Fields</Tabs.Trigger>
          <Tabs.Trigger value="views">Item views</Tabs.Trigger>
        </Tabs.List>
        <Tabs.Content value="fields" className="section-settings-tab" forceMount>
          <div className="field-panel">
            <div className="panel-heading">
              <h2>{section?.name} fields</h2>
              <button className="secondary" onClick={() => onAddField()}>
                <Plus size={16} />
                Add field
              </button>
            </div>
            {fields.map((field) => (
              <div className="field-row" key={field.id}>
                <div>
                  <b>{field.label}</b>
                  <code>{field.id}</code>
                </div>
                <span>{field.type}</span>
                <span>{field.required ? 'Required' : 'Optional'}</span>
              </div>
            ))}
            <p className="muted text-sm mt-5">
              New sections automatically appear in navigation and agent permissions. Access starts
              disabled.
            </p>
          </div>
        </Tabs.Content>
        <Tabs.Content value="views" className="section-settings-tab" forceMount>
          <ItemViewsEditor section={section} catalog={catalog} />
        </Tabs.Content>
      </Tabs.Root>
    </div>
  );
}
