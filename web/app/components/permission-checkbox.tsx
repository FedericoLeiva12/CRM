import * as Checkbox from '@radix-ui/react-checkbox';
import { Check } from 'lucide-react';
export function CheckControl({
  name,
  checked,
  label,
}: {
  name: string;
  checked: boolean;
  label: string;
}) {
  return (
    <Checkbox.Root name={name} defaultChecked={checked} className="checkbox" aria-label={label}>
      <Checkbox.Indicator>
        <Check size={14} />
      </Checkbox.Indicator>
    </Checkbox.Root>
  );
}
