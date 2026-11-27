import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { ApiError, post } from "../api";

export interface ActionDialogProps<T> {
  title: string;
  // What will happen, in plain words; shown above the form.
  summary: ReactNode;
  confirmLabel: string;
  danger?: boolean;
  path: string;
  // Extra fields beside the reason, merged into the request body.
  body?: Record<string, unknown>;
  children?: ReactNode;
  // Disables the confirm button (for example while a field is invalid).
  blocked?: string | null;
  onDone: (result: T) => void;
  onClose: () => void;
}

// ActionDialog confirms one admin change. Every change needs a reason, which
// goes in the audit log. Escape or Cancel closes it; focus stays inside.
export function ActionDialog<T>(p: ActionDialogProps<T>) {
  const [reason, setReason] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const ref = useRef<HTMLDivElement>(null);
  const titleId = useId();
  const reasonId = useId();

  useEffect(() => {
    const before = document.activeElement as HTMLElement | null;
    ref.current?.querySelector<HTMLElement>("textarea, input, select, button")?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !busy) p.onClose();
      if (e.key !== "Tab" || !ref.current) return;
      const items = ref.current.querySelectorAll<HTMLElement>("button, input, select, textarea, a[href]");
      const first = items[0];
      const last = items[items.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("keydown", onKey);
      before?.focus();
    };
  }, []);

  const trimmed = reason.trim();
  const submit = async () => {
    setBusy(true);
    setError(null);
    try {
      const out = await post<T>(p.path, { ...p.body, reason: trimmed });
      p.onDone(out);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "That didn't work. Try again.");
      setBusy(false);
    }
  };

  return (
    <div className="scrim" onMouseDown={(e) => e.target === e.currentTarget && !busy && p.onClose()}>
      <div className="panel dialog" role="dialog" aria-modal="true" aria-labelledby={titleId} ref={ref}>
        <h2 id={titleId}>{p.title}</h2>
        <div className="dialog-summary">{p.summary}</div>
        {p.children}
        <label className="field" htmlFor={reasonId}>
          <span>Reason</span>
          <textarea
            id={reasonId}
            className="input"
            rows={2}
            maxLength={300}
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="Why? This goes in the audit log."
          />
        </label>
        {error && (
          <div className="notice" data-tone="hot" role="alert">
            {error}
          </div>
        )}
        <div className="dialog-actions">
          <button type="button" className="btn" onClick={p.onClose} disabled={busy}>
            Cancel
          </button>
          <button
            type="button"
            className={p.danger ? "btn btn-danger-solid" : "btn btn-primary"}
            disabled={busy || trimmed.length < 3 || !!p.blocked}
            title={p.blocked ?? (trimmed.length < 3 ? "Add a reason first" : undefined)}
            onClick={() => void submit()}
          >
            {busy ? "Working…" : p.confirmLabel}
          </button>
        </div>
      </div>
    </div>
  );
}

// Flash shows the result of the last change at the top of a page.
export function Flash({ message, onClose }: { message: string | null; onClose: () => void }) {
  if (!message) return null;
  return (
    <div className="notice flash" role="status">
      <span>{message}</span>
      <button type="button" className="link-btn" onClick={onClose}>
        Dismiss
      </button>
    </div>
  );
}
