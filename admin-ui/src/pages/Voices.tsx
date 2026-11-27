import { useEffect, useState, type FormEvent } from "react";
import type { JobRow, VoiceRow } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { ActionDialog, Flash } from "../components/ActionDialog";
import { ago, n, shortDay } from "../format";
import { go, type Route } from "../router";

interface VoicesPage {
  items: VoiceRow[];
  total: number;
  page: number;
  page_size: number;
  jobs: JobRow[];
}

const kindName = { voice: "Voice", dict: "Dictionary" } as const;

function jobState(v: VoiceRow) {
  if (v.job_status === "pending" || v.job_status === "running") return <span className="pill">Deleting…</span>;
  if (v.job_status === "failed")
    return (
      <span className="pill" data-tone="hot" title={v.job_error}>
        Delete failed
      </span>
    );
  return null;
}

export function Voices({ route }: { route: Route }) {
  const owner = route.params.get("owner") ?? "";
  const kind = route.params.get("kind") ?? "";
  const pageNo = Number(route.params.get("page") ?? "1") || 1;
  const [draft, setDraft] = useState(owner);
  const [target, setTarget] = useState<VoiceRow | null>(null);
  const [flash, setFlash] = useState<string | null>(null);
  const qs = new URLSearchParams({ page: String(pageNo), ...(owner ? { owner } : {}), ...(kind ? { kind } : {}) });
  // While a deletion is under way, check every 4 seconds.
  const [busy, setBusy] = useState(false);
  const { data, error, reload } = useLoad<VoicesPage>(`/api/voices?${qs}`, busy ? 4000 : 0);
  const working = !!data?.jobs.some((j) => j.status === "pending" || j.status === "running");
  useEffect(() => setBusy(working), [working]);

  const nav = (next: Record<string, string>) => {
    const p = new URLSearchParams({ ...(owner ? { owner } : {}), ...(kind ? { kind } : {}), page: "1", ...next });
    go(`/voices?${p}`);
  };
  const search = (e: FormEvent) => {
    e.preventDefault();
    nav({ owner: draft.trim() });
  };
  const pages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1;

  return (
    <div className="stack">
      <Flash message={flash} onClose={() => setFlash(null)} />
      <section className="panel">
        <form className="toolbar" onSubmit={search} role="search">
          <label className="visually-hidden" htmlFor="owner-search">
            Made by (Telegram user ID)
          </label>
          <input id="owner-search" className="input num" inputMode="numeric" placeholder="Made by (Telegram user ID)" value={draft} onChange={(e) => setDraft(e.target.value)} />
          <button className="btn" type="submit">
            Find
          </button>
          {owner && (
            <button className="link-btn" type="button" onClick={() => (setDraft(""), nav({ owner: "" }))}>
              Show everyone's
            </button>
          )}
          <label className="visually-hidden" htmlFor="kind-filter">
            Show
          </label>
          <select id="kind-filter" className="select" value={kind} onChange={(e) => nav({ kind: e.target.value })} style={{ marginLeft: "auto" }}>
            <option value="">Voices and dictionaries</option>
            <option value="voice">Voices</option>
            <option value="dict">Dictionaries</option>
          </select>
        </form>
        {error && !data ? (
          <div className="panel-body">
            <Failed message={error} retry={reload} />
          </div>
        ) : !data ? (
          <div className="panel-body">
            <Loading />
          </div>
        ) : data.items.length === 0 ? (
          <p className="empty">{owner ? `User ${owner} hasn't made any voices or dictionaries.` : "Nobody has made a voice or dictionary yet."}</p>
        ) : (
          <>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th scope="col">Name</th>
                    <th scope="col">Kind</th>
                    <th scope="col">Made by</th>
                    <th scope="col">Made</th>
                    <th scope="col">
                      <span className="visually-hidden">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.items.map((v) => (
                    <tr key={`${v.kind}:${v.id}`}>
                      <td>
                        <div style={{ fontWeight: 550 }}>{v.name || <span className="muted">Name not synced yet</span>}</div>
                        <div className="muted mono">{v.id}</div>
                      </td>
                      <td>{kindName[v.kind]}</td>
                      <td className="num">
                        <a href={`#/users/${v.owner_id}`}>{v.owner_id}</a>
                      </td>
                      <td className="muted">{shortDay(v.created_at.slice(0, 10))}</td>
                      <td>
                        {jobState(v) ?? (
                          <button className="btn btn-small btn-danger" onClick={() => setTarget(v)}>
                            Delete
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="pager">
              <span className="muted num">{n(data.total)} made through Ellie</span>
              <span style={{ display: "flex", gap: 8 }}>
                <button className="btn" disabled={pageNo <= 1} onClick={() => nav({ page: String(pageNo - 1) })}>
                  Previous
                </button>
                <button className="btn" disabled={pageNo >= pages} onClick={() => nav({ page: String(pageNo + 1) })}>
                  Next
                </button>
              </span>
            </div>
          </>
        )}
      </section>

      {data && data.jobs.length > 0 && (
        <section className="panel">
          <div className="panel-head">
            <h2>Recent deletions</h2>
            <span className="muted">Ellie does these within a few seconds</span>
          </div>
          <ul className="list" style={{ marginTop: 8 }}>
            {data.jobs.map((j) => (
              <li key={j.id}>
                <div>
                  <div className="what mono">{j.target}</div>
                  <div className="meta">
                    {j.kind === "delete_voice" ? "Voice" : "Dictionary"} requested by admin {j.requested_by}, {ago(j.created_at)}
                    {j.status === "failed" && j.error ? `. ${j.error}` : ""}
                  </div>
                </div>
                <span className="pill" data-tone={j.status === "done" ? "ok" : j.status === "failed" ? "hot" : undefined}>
                  {{ pending: "Waiting for Ellie", running: "Deleting", done: "Deleted", failed: "Failed" }[j.status]}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      {target && (
        <ActionDialog<{ already_requested: boolean; bot_online: boolean }>
          title={`Delete ${target.name || target.id}`}
          summary={
            <>
              This deletes the {target.kind === "voice" ? "voice" : "dictionary"} from Cartesia for good. Anyone using it goes back to the default. Ellie
              does it within a few seconds.
            </>
          }
          confirmLabel={`Delete ${target.kind === "voice" ? "voice" : "dictionary"}`}
          danger
          path={`/api/voices/${target.kind}/${encodeURIComponent(target.id)}/delete`}
          onClose={() => setTarget(null)}
          onDone={(out) => {
            setTarget(null);
            setFlash(
              out.already_requested
                ? "That deletion was already on its way."
                : out.bot_online
                  ? "Deleting now. It disappears from the list once Ellie is done."
                  : "Queued. Ellie is offline right now, so it will run as soon as Ellie is back.",
            );
            setBusy(true);
            reload();
          }}
        />
      )}
    </div>
  );
}
