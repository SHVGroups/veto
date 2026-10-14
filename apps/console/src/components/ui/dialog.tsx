"use client";

import { useEffect, useRef, useState, type ReactNode } from "react";
import { errorMessage } from "@/lib/api";
import { cx } from "@/lib/cx";
import { Icon } from "../icons";
import { Button } from "./button";
import { Field, Input } from "./form";

/**
 * A modal built on the native dialog element, which provides focus
 * trapping, Escape to close and an inert background.
 */
export function Dialog({
  open,
  onClose,
  title,
  description,
  children,
  footer,
  width = "md",
}: {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  description?: ReactNode;
  children?: ReactNode;
  footer?: ReactNode;
  width?: "sm" | "md" | "lg" | "xl";
}) {
  const ref = useRef<HTMLDialogElement>(null);

  useEffect(() => {
    const d = ref.current;
    if (!d) return;
    if (open && !d.open) d.showModal();
    if (!open && d.open) d.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      onClose={onClose}
      onCancel={(e) => {
        e.preventDefault();
        onClose();
      }}
      onClick={(e) => {
        if (e.target === ref.current) onClose();
      }}
      className={cx(
        "m-auto max-h-[min(88dvh,820px)] w-[calc(100%-2rem)] overflow-hidden rounded-[20px] bg-surface p-0 text-ink shadow-pop backdrop:bg-[rgb(10_11_13/0.42)] backdrop:backdrop-blur-[3px]",
        { sm: "max-w-sm", md: "max-w-lg", lg: "max-w-2xl", xl: "max-w-4xl" }[width],
      )}
    >
      {open && (
        <div className="flex max-h-[inherit] flex-col">
          <div className="flex items-start justify-between gap-4 px-6 pt-5 pb-3">
            <div className="min-w-0">
              <h2 className="text-lg text-ink">{title}</h2>
              {description && <p className="mt-1 text-sm text-ink-2">{description}</p>}
            </div>
            <button
              type="button"
              onClick={onClose}
              aria-label="Close"
              className="-mr-2 inline-flex size-8 shrink-0 items-center justify-center rounded-[9px] text-ink-3 hover:bg-hover hover:text-ink"
            >
              <Icon.Close size={16} />
            </button>
          </div>
          {children && <div className="scroll-quiet min-h-0 flex-1 overflow-y-auto px-6 pb-5">{children}</div>}
          {footer && <div className="flex justify-end gap-2 border-t border-line bg-sunken px-6 py-3.5">{footer}</div>}
        </div>
      )}
    </dialog>
  );
}

interface ConfirmProps {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  body: ReactNode;
  confirmLabel: string;
  tone?: "danger" | "primary";
  onConfirm: () => Promise<unknown>;
  typeToConfirm?: string;
}

/**
 * Asks before doing something that cannot be undone. The action runs inside
 * the dialog so failures are shown where the person is looking. Its state
 * is created fresh each time it opens.
 */
export function ConfirmDialog(props: ConfirmProps) {
  return props.open ? <ConfirmBody {...props} /> : null;
}

function ConfirmBody({ onClose, title, body, confirmLabel, tone = "danger", onConfirm, typeToConfirm }: ConfirmProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [typed, setTyped] = useState("");

  async function run() {
    setBusy(true);
    setError(null);
    try {
      await onConfirm();
      onClose();
    } catch (err) {
      setError(errorMessage(err));
      setBusy(false);
    }
  }

  const blocked = typeToConfirm !== undefined && typed !== typeToConfirm;

  return (
    <Dialog
      open
      onClose={onClose}
      title={title}
      width="sm"
      footer={
        <>
          <Button variant="ghost" onClick={onClose}>
            Cancel
          </Button>
          <Button variant={tone} busy={busy} disabled={blocked} onClick={run}>
            {confirmLabel}
          </Button>
        </>
      }
    >
      <div className="space-y-4 text-sm text-ink-2">
        <div>{body}</div>
        {typeToConfirm !== undefined && (
          <Field
            label={
              <>
                Type <code className="text-ink">{typeToConfirm}</code> to confirm
              </>
            }
          >
            {(id) => <Input id={id} value={typed} onChange={(e) => setTyped(e.target.value)} autoComplete="off" />}
          </Field>
        )}
        {error && <p className="text-stop">{error}</p>}
      </div>
    </Dialog>
  );
}
