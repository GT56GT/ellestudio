import { useEffect, useState } from "react";
import type { AuditEntry } from "../types";
import { ApiError, get } from "../api";
import { Failed, Loading } from "../components/States";
import { dateTime, device } from "../format";

const methods: Record<string, string> = { login_widget: "in a browser", mini_app: "in Telegram" };

// describe turns an audit entry into a sentence.
function describe(e: AuditEntry) {
  const who = (e.detail.name as string) || `Admin ${e.admin_id}`;
  switch (e.action) {
    case "login":
      return `${who} signed in ${methods[e.detail.method as string] ?? ""}`.trim();
    case "login_refused":
      return `${who} (${e.admin_id}) tried to sign in but isn't an admin`;
    case "logout":
      return `Admin ${e.admin_id} signed out`;
    case "session_revoked":
      return `Admin ${e.admin_id} signed out a device`;
    case "sessions_revoked":
      return `Admin ${e.admin_id} signed out ${e.detail.count ?? "all"} other devices`;
    default:
      return `Admin ${e.admin_id}: ${e.action} ${e.target}`.trim();
  }
}

export function Audit() {
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [more, setMore] = useState(false);
  const [busy, setBusy] = useState(false);

  const load = async (before = 0) => {
    setBusy(true);
    try {
      const out = await get<{ entries: AuditEntry[]; page_size: number }>(`/api/audit?before=${before}`);
      setEntries((prev) => (before && prev ? [...prev, ...out.entries] : out.entries));
      setMore(out.entries.length === out.page_size);
      setError(null);
    } catch (e) {
      setError(e instanceof ApiError ? e.message : "Couldn't load the audit log.");
    } finally {
      setBusy(false);
    }
  };
  useEffect(() => {
    void load();
  }, []);

  if (error && !entries) return <Failed message={error} retry={() => void load()} />;
  if (!entries) return <Loading />;
  return (
    <section className="panel">
      {entries.length === 0 ? (
        <p className="empty">No admin actions yet. Sign-ins and changes will show up here.</p>
      ) : (
        <ul className="list">
          {entries.map((e) => (
            <li key={e.id}>
              <div>
                <div className="what">{describe(e)}</div>
                <div className="meta">
                  {[e.ip && `From ${e.ip}`, typeof e.detail.user_agent === "string" && device(e.detail.user_agent)].filter(Boolean).join(", ")}
                </div>
              </div>
              <time className="meta num" dateTime={e.at}>
                {dateTime(e.at)}
              </time>
            </li>
          ))}
        </ul>
      )}
      {more && (
        <div className="pager">
          <span />
          <button className="btn" disabled={busy} onClick={() => void load(entries[entries.length - 1].id)}>
            Show older
          </button>
        </div>
      )}
    </section>
  );
}
