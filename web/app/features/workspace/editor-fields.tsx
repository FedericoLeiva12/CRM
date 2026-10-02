import type { CRMRecord, Field } from '../../types/crm';

function RecordInput({
  field,
  value,
}: {
  field: Field;
  value: CRMRecord['data'][string] | undefined;
}) {
  const name = `field:${field.id}`;
  if (field.type === 'boolean')
    return <input type="checkbox" name={name} defaultChecked={Boolean(value)} />;
  if (field.id === 'notes')
    return <textarea name={name} defaultValue={String(value ?? '')} required={field.required} />;
  return (
    <input
      name={name}
      type={field.type}
      step={field.type === 'number' ? 'any' : undefined}
      required={field.required}
      defaultValue={String(value ?? '')}
    />
  );
}
export function RecordFields({ fields, record }: { fields: Field[]; record: CRMRecord | null }) {
  return (
    <>
      <input type="hidden" name="id" value={record?.id || ''} />
      <div className="form-fields">
        {fields.map((field) => (
          <label key={field.id}>
            {field.label}
            {field.required && <span className="required"> *</span>}
            <RecordInput field={field} value={record?.data[field.id]} />
          </label>
        ))}
      </div>
    </>
  );
}
export function CustomFieldInputs() {
  return (
    <>
      <label>
        Field label
        <input name="label" required maxLength={80} placeholder="e.g. Industry" />
      </label>
      <label>
        Field identifier
        <input name="key" required pattern="[a-z][a-z0-9_]{0,47}" placeholder="e.g. industry" />
        <small>Lowercase letters, numbers, and underscores.</small>
      </label>
      <label>
        Field type
        <select name="type">
          <option value="text">Text</option>
          <option value="email">Email</option>
          <option value="number">Number</option>
          <option value="date">Date</option>
          <option value="boolean">Checkbox</option>
        </select>
      </label>
      <label className="inline-label">
        <input type="checkbox" name="required" />
        Required field
      </label>
      <p className="muted text-sm">
        Fields added to sections with existing records must be optional.
      </p>
    </>
  );
}
export function SectionInputs() {
  return (
    <>
      <label>
        Section name
        <input name="name" required maxLength={80} placeholder="e.g. Employees" />
      </label>
      <label>
        Section identifier
        <input name="key" required pattern="[a-z][a-z0-9_]{0,47}" placeholder="e.g. employees" />
      </label>
      <p className="muted">
        A required Name field is added automatically. Add more fields from settings.
      </p>
    </>
  );
}
export function AgentInputs() {
  return (
    <>
      <label>
        Agent name
        <input name="name" required maxLength={80} placeholder="e.g. Sales assistant" />
      </label>
      <p className="muted">
        Your token will be shown once. Section access and schema management start disabled.
      </p>
    </>
  );
}
export function PasswordInputs() {
  return (
    <>
      <label>
        Current password
        <input name="current" type="password" autoComplete="current-password" required />
      </label>
      <label>
        New password
        <input
          name="password"
          type="password"
          autoComplete="new-password"
          minLength={14}
          maxLength={72}
          required
        />
      </label>
      <p className="muted">Use 14–72 characters. Changing your password signs out all sessions.</p>
    </>
  );
}
