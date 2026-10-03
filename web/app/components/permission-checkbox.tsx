import * as Checkbox from '@radix-ui/react-checkbox';
import { Check } from 'lucide-react';
export function CheckControl({
  name,
  checked,
  label,
  disabled = false,
  onChange,
}: {
  name: string;
  checked: boolean;
  label: string;
  disabled?: boolean;
  onChange?: (checked: boolean) => void;
}) {
  return (
    <Checkbox.Root
      name={name}
      checked={onChange ? checked : undefined}
      defaultChecked={onChange ? undefined : checked}
      onCheckedChange={onChange ? (value) => onChange(value === true) : undefined}
      disabled={disabled}
      className="checkbox"
      aria-label={label}
    >
      <Checkbox.Indicator>
        <Check size={14} />
      </Checkbox.Indicator>
    </Checkbox.Root>
  );
}
