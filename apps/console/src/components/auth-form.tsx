"use client";

import type { FormEvent, ReactNode } from "react";
import { Button } from "./ui/button";

/** A sign-in style form: heading, fields, one primary action, an error line. */
export function AuthForm({
  title,
  description,
  children,
  submitLabel,
  busy,
  error,
  onSubmit,
  footer,
}: {
  title: ReactNode;
  description?: ReactNode;
  children: ReactNode;
  submitLabel: string;
  busy: boolean;
  error: string | null;
  onSubmit: () => void;
  footer?: ReactNode;
}) {
  function submit(e: FormEvent) {
    e.preventDefault();
    onSubmit();
  }
  return (
    <form onSubmit={submit} noValidate>
      <h1 className="text-2xl text-ink">{title}</h1>
      {description && <p className="mt-2 text-base text-ink-2">{description}</p>}
      <div className="mt-8 space-y-5">{children}</div>
      {error && (
        <p role="alert" className="mt-5 rounded-[12px] bg-stop-wash px-3.5 py-2.5 text-sm text-stop">
          {error}
        </p>
      )}
      <Button type="submit" variant="primary" size="lg" busy={busy} className="mt-7 w-full">
        {submitLabel}
      </Button>
      {footer && <div className="mt-6 text-sm text-ink-3">{footer}</div>}
    </form>
  );
}
