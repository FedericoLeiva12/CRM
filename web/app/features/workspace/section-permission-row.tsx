import { useState } from 'react';
import { CheckControl } from '../../components/permission-checkbox';
import type { Permission } from '../../types/crm';

// The parent keys this row by persisted grants, so revalidation replaces local
// drafts only when the server's permission values actually change.
export function SectionPermissionRow({
  permission,
  name,
  disabled,
}: {
  permission: Permission;
  name: string;
  disabled: boolean;
}) {
  const [read, setRead] = useState(permission.read);
  const [write, setWrite] = useState(permission.write);
  const [canDelete, setCanDelete] = useState(permission.delete);
  function changeRead(enabled: boolean) {
    setRead(enabled);
    if (!enabled) {
      setWrite(false);
      setCanDelete(false);
    }
  }
  function changeWrite(enabled: boolean) {
    setWrite(enabled);
    if (enabled) setRead(true);
    else setCanDelete(false);
  }
  return (
    <div className="permission-row">
      <span>{name}</span>
      <CheckControl
        name={`${permission.section_id}:read`}
        checked={read}
        onChange={changeRead}
        disabled={disabled}
        label={`Read ${name}`}
      />
      <CheckControl
        name={`${permission.section_id}:write`}
        checked={write}
        onChange={changeWrite}
        disabled={disabled}
        label={`Write ${name}`}
      />
      <CheckControl
        name={`${permission.section_id}:delete`}
        checked={canDelete}
        onChange={setCanDelete}
        disabled={disabled || !write}
        label={`Delete ${name} records`}
      />
    </div>
  );
}
