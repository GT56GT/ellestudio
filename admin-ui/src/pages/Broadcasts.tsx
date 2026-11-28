import { useEffect, useState } from "react";
import type { BroadcastRow } from "../types";
import { ApiError, post } from "../api";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { ActionDialog, Flash } from "../components/ActionDialog";
import { ago, n } from "../format";

interface BroadcastsData {
  broadcasts: BroadcastRow[];
  audience: Record<"everyone" | "active_30d", number>;
}

const MAX = 3500;
const audiences = [
  { value: "everyone", label: "Everyone who uses Ellie's private chat" },
  { value: "active_30d", label: "Only people active in the last 30 days" },
] as const;

const statusLabel: Record<BroadcastRow["status"], string> = {
  queued: "Waiting for Ellie",
  sending: "Sending",
  done: "Sent",
  cancelled: "Cancelled",
};

export function Broadcasts() {
  const [busy, setBusy] = useState(false);
  const { data, error, reload } = useLoad<BroadcastsData>("/api/broadcasts", busy ? 4000 : 0);
  const [text, setText] = useState("");
  const [audience, setAudience] = useState<"everyone" | "active_30d">("everyone");
  const [dialog, setDialog] = useState<"send" | { cancel: BroadcastRow } | null>(null);
  const [flash, setFlash] = useState<string | null>(null);
  const [testing, setTesting] = useState(false);
  const [testNote, setTestNote] = useState<string | null>(null);

  const open = data?.broadcasts.find((b) => b.status === "queued" || b.status === "sending");
  useEffect(() => setBusy(!!open), [open]);

  if (error && !data) return <Failed message={error} retry={reload} />;
  if (!data) return <Loading rows={4} />;

  const length = [...text].length;
  const tooLong = length > MAX;
  const reach = data.audience[audience];

  const sendTest = async () => {
    setTesting(true);
    setTestNote(null);
    try {
      await post("/api/broadcasts/test", { text });
      setTestNote("Sent to you in Telegram. Check how it looks before sending it to everyone.");
    } catch (e) {
      setTestNote(e instanceof ApiError ? e.message : "Couldn't send the test.");
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="stack">
      <Flash message={flash} onClose={() => setFlash(null)} />
      <section className="panel">
        <div className="panel-head">
          <h2>New announcement</h2>
          <span className="muted" style={{ fontSize: 14 }}>
            Every message has a “Stop these messages” button
          </span>
        </div>
        <div className="panel-body stack" style={{ gap: 14 }}>
          <label className="field">
            <span>Message</span>
            <textarea
              className="input"
              rows={6}
              value={text}
              onChange={(e) => setText(e.target.value)}
              placeholder="What's new? Plain text; links work."
              aria-describedby="length-note"
            />
            <span id="length-note" className="muted num" style={{ fontWeight: 400, color: tooLong ? "var(--hot)" : undefined }}>
              {n(length)} of {n(MAX)} characters
            </span>
          </label>
          <fieldset className="choices" style={{ border: 0, padding: 0, margin: 0, flexDirection: "column", alignItems: "flex-start" }}>
            <legend className="field" style={{ marginBottom: 6 }}>
              Who gets it
            </legend>
            {audiences.map((a) => (
              <label key={a.value}>
                <input type="radio" name="audience" checked={audience === a.value} onChange={() => setAudience(a.value)} />
                {a.label} <span className="muted num">({n(data.audience[a.value])})</span>
              </label>
            ))}
          </fieldset>
          {text.trim() && (
            <div>
              <div className="muted" style={{ fontSize: 13, marginBottom: 6 }}>
                Preview
              </div>
              <div className="preview-bubble">
                <div style={{ whiteSpace: "pre-wrap" }}>📣 {text}</div>
                <div className="preview-button">🔕 Stop these messages</div>
              </div>
            </div>
          )}
          {testNote && <div className="notice">{testNote}</div>}
          <div className="actions-row" style={{ justifyContent: "flex-end" }}>
            <button className="btn" disabled={!text.trim() || tooLong || testing} onClick={() => void sendTest()}>
              {testing ? "Sending…" : "Send a test to me"}
            </button>
            <button
              className="btn btn-primary"
              disabled={!text.trim() || tooLong || !!open || reach === 0}
              title={open ? "Another announcement is still going out" : undefined}
              onClick={() => setDialog("send")}
            >
              Send to {n(reach)} {reach === 1 ? "person" : "people"}
            </button>
          </div>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>Sent announcements</h2>
          <span className="muted" style={{ fontSize: 14 }}>
            Ellie sends about 10 a second
          </span>
        </div>
        {data.broadcasts.length === 0 ? (
          <p className="empty">No announcements yet.</p>
        ) : (
          <ul className="list" style={{ marginTop: 8 }}>
            {data.broadcasts.map((b) => (
              <li key={b.id}>
                <div style={{ minWidth: 0, flex: 1 }}>
                  <div className="what" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap", maxWidth: 560 }}>
                    {b.text}
                  </div>
                  <div className="meta num">
                    {b.audience === "everyone" ? "Everyone" : "Active in 30 days"}, by admin {b.created_by}, {ago(b.created_at)}.{" "}
                    {b.total > 0 && `${n(b.sent)} of ${n(b.total)} delivered${b.failed ? `, ${n(b.failed)} couldn't be reached` : ""}.`}
                  </div>
                  {b.status === "sending" && b.total > 0 && (
                    <div className="bar" style={{ marginTop: 6 }} aria-hidden="true">
                      <i style={{ width: `${Math.min(100, ((b.sent + b.failed) / b.total) * 100)}%` }} />
                    </div>
                  )}
                </div>
                <span style={{ display: "flex", gap: 8, alignItems: "center" }}>
                  <span className="pill" data-tone={b.status === "done" ? "ok" : b.status === "cancelled" ? "hot" : undefined}>
                    {statusLabel[b.status]}
                  </span>
                  {(b.status === "queued" || b.status === "sending") && (
                    <button className="btn btn-small btn-danger" onClick={() => setDialog({ cancel: b })}>
                      Cancel
                    </button>
                  )}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>

      {dialog === "send" && (
        <ActionDialog<{ recipients: number }>
          title="Send announcement"
          summary={
            <>
              This goes to <b>{n(reach)}</b> {reach === 1 ? "person" : "people"} in Telegram and can't be unsent. Ellie sends about 10 a second, so it takes
              about {Math.max(1, Math.round(reach / 10 / 60))} min. You can cancel it while it's going out.
            </>
          }
          confirmLabel={`Send to ${n(reach)}`}
          danger
          path="/api/broadcasts"
          body={{ text, audience }}
          onClose={() => setDialog(null)}
          onDone={(out) => {
            setDialog(null);
            setText("");
            setTestNote(null);
            setFlash(`Sending to ${n(out.recipients)} people. Progress shows below.`);
            setBusy(true);
            reload();
          }}
        />
      )}
      {dialog && typeof dialog === "object" && (
        <ActionDialog
          title="Cancel announcement"
          summary={`Ellie stops within a few seconds. ${n(dialog.cancel.sent)} people already got it.`}
          confirmLabel="Stop sending"
          danger
          path={`/api/broadcasts/${dialog.cancel.id}/cancel`}
          onClose={() => setDialog(null)}
          onDone={() => {
            setDialog(null);
            setFlash("Cancelled. Nobody else will get it.");
            reload();
          }}
        />
      )}
    </div>
  );
}
