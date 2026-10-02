import { Plus } from 'lucide-react';
import type { Section } from '../../types/crm';
interface Props {
  sections: Section[];
  section: Section;
  onSelectSection: (id: string) => void;
  onAddField: () => void;
}
export function FieldsView({ sections, section, onSelectSection, onAddField }: Props) {
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
    </div>
  );
}
