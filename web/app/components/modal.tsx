import * as Dialog from '@radix-ui/react-dialog';
import { X } from 'lucide-react';
export function Modal({
  title,
  children,
  open,
  wide,
  onOpenChange,
}: {
  title: string;
  children: React.ReactNode;
  open: boolean;
  wide?: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  return (
    <Dialog.Root open={open} onOpenChange={onOpenChange}>
      <Dialog.Portal>
        <Dialog.Overlay className="overlay" />
        <Dialog.Content
          className={wide ? 'dialog wide' : 'dialog'}
          onEscapeKeyDown={(event) => {
            // Escape closes an open suggestion list first, not the whole dialog.
            if (event.target instanceof Element && event.target.closest('[aria-expanded="true"]')) {
              event.preventDefault();
            }
          }}
        >
          <div className="dialog-heading">
            <Dialog.Title>{title}</Dialog.Title>
            <Dialog.Close className="icon-button" aria-label="Close dialog">
              <X size={20} />
            </Dialog.Close>
          </div>
          <Dialog.Description className="sr-only">
            Manage your workspace information.
          </Dialog.Description>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
