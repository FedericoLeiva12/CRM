import type { ItemViewConfigField } from '../../types/crm';

// Configuration fields come from the Go catalog. A future system prompt can be
// a textarea setting without adding a section-specific form or storage column.
export function ViewConfigFields({
  fields,
  values,
  disabled,
  onChange,
}: {
  fields: ItemViewConfigField[];
  values: Record<string, string>;
  disabled: boolean;
  onChange: (key: string, value: string) => void;
}) {
  return (
    <div className="view-config-fields">
      {fields.map((field) => (
        <label key={field.key}>
          {field.label}
          {field.type === 'textarea' ? (
            <textarea
              value={values[field.key] || ''}
              disabled={disabled}
              required={field.required}
              maxLength={field.max_length || undefined}
              onChange={(event) => onChange(field.key, event.target.value)}
            />
          ) : (
            <input
              value={values[field.key] || ''}
              disabled={disabled}
              required={field.required}
              maxLength={field.max_length || undefined}
              onChange={(event) => onChange(field.key, event.target.value)}
            />
          )}
        </label>
      ))}
    </div>
  );
}
