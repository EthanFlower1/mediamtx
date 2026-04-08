import { useEffect, useRef, type ReactNode } from 'react';

// KAI-321: Minimal accessible modal.
//
// Not a substitute for a full component library Dialog, but enough to
// meet WCAG 2.1 AA for this scope: focus trap, Escape closes, initial
// focus on the first focusable, role="dialog", aria-modal, labelled
// by heading id. shadcn/ui Dialog will replace this once the
// component set is fully wired in a later ticket.

export interface ModalProps {
  open: boolean;
  onClose: () => void;
  titleId: string;
  descriptionId?: string;
  children: ReactNode;
  testId?: string;
}

const FOCUSABLE =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function Modal({
  open,
  onClose,
  titleId,
  descriptionId,
  children,
  testId,
}: ModalProps): JSX.Element | null {
  const ref = useRef<HTMLDivElement | null>(null);

  useEffect(() => {
    if (!open) return;
    const node = ref.current;
    if (!node) return;

    const focusables = Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE));
    focusables[0]?.focus();

    function handleKey(e: KeyboardEvent): void {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
        return;
      }
      if (e.key === 'Tab' && node) {
        const list = Array.from(node.querySelectorAll<HTMLElement>(FOCUSABLE));
        if (list.length === 0) return;
        const first = list[0]!;
        const last = list[list.length - 1]!;
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault();
          last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault();
          first.focus();
        }
      }
    }

    document.addEventListener('keydown', handleKey);
    return () => document.removeEventListener('keydown', handleKey);
  }, [open, onClose]);

  if (!open) return null;
  return (
    <div
      className="modal-backdrop"
      style={{
        position: 'fixed',
        inset: 0,
        background: 'rgba(0,0,0,0.4)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 50,
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={descriptionId}
        ref={ref}
        data-testid={testId}
        className="modal"
        style={{
          background: '#fff',
          padding: 16,
          minWidth: 480,
          maxWidth: '90vw',
          maxHeight: '90vh',
          overflow: 'auto',
          borderRadius: 8,
        }}
      >
        {children}
      </div>
    </div>
  );
}
