import { useState } from "react";
import type { Session } from "../types";
import { ApiError, post } from "../api";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { ago, dateTime, device } from "../format";

export function Sessions({ onSignedOut }: { onSignedOut: () => void }) {
  const { data, error, reload } = useLoad<{ sessions: Session[] }>("/api/sessions");
  const [busy, setBusy] = useState<string | null>(null);
  const [failure, setFailure] = useState<string | null>(null);

  const act = async (key: string, path: string, endsThisSession: boolean) => {
    setBusy(key);
    setFailure(null);
    try {
      await post(path);
      if (endsThisSession) onSignedOut();
      else reload();
    } catch (e) {
      setFailure(e instanceof ApiError ? e.message : "That didn't work. Try again.");
    } finally {
      setBusy(null);
    }
  };

  if (error && !data) return <Failed message={error} retry={reload} />;
  if (!data) return <Loading />;
  const others = data.sessions.filter((s) => !s.current).length;

  return (
    <div className="stack">
      {failure && <Failed message={failure} />}
      <section className="panel">
        <div className="panel-head">
          <h2>Signed-in devices</h2>
          <button className="btn btn-danger" disabled={!others || busy !== null} onClick={() => act("others", "/api/sessions/revoke-others", false)}>
            Sign out other devices
          </button>
        </div>
        <ul className="list" style={{ marginTop: 12 }}>
          {data.sessions.map((s) => (
            <li key={s.id}>
              <div>
                <div className="what">
                  {device(s.user_agent)} {s.current && <span className="pill" data-tone="ok">This device</span>}
                </div>
                <div className="meta">
                  Signed in {dateTime(s.created_at)}, active {ago(s.last_seen)}
                  {s.ip ? `, from ${s.ip}` : ""}. Ends {dateTime(s.expires_at)}.
                </div>
              </div>
              <button className="btn" disabled={busy !== null} onClick={() => act(s.id, `/api/sessions/${s.id}/revoke`, s.current)}>
                {s.current ? "Sign out" : "Sign out this device"}
              </button>
            </li>
          ))}
        </ul>
      </section>
      <p className="muted" style={{ margin: 0, fontSize: 14 }}>
        Sessions end after 12 hours, or after 2 hours without activity. Removing someone from ADMIN_USER_IDS signs them out everywhere at once.
      </p>
    </div>
  );
}
