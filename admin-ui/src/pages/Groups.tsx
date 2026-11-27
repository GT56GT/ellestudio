import { useState } from "react";
import type { GroupRow, GroupSettings } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { ActionDialog, Flash } from "../components/ActionDialog";
import { minutes, n, shortDay } from "../format";
import { go, type Route } from "../router";

interface GroupsPage {
  groups: GroupRow[];
  total: number;
  page: number;
  page_size: number;
}

function status(g: GroupRow) {
  if (g.blocked) return <span className="pill" data-tone="hot">Blocked</span>;
  if (g.status === "member") return <span className="pill" data-tone="ok">In the group</span>;
  if (g.status === "guest") return <span className="pill">Guest replies only</span>;
  return <span className="pill">Left</span>;
}

const switches: { key: keyof GroupSettings; label: string }[] = [
  { key: "paused", label: "Pause Ellie here" },
  { key: "admins_only", label: "Only group admins can use Ellie" },
  { key: "no_speak", label: "Turn off reading aloud" },
  { key: "no_transcribe", label: "Turn off transcription" },
];

type Open = { kind: "block" | "unblock" | "settings"; group: GroupRow } | null;

export function Groups({ route }: { route: Route }) {
  const pageNo = Number(route.params.get("page") ?? "1") || 1;
  const { data, error, reload } = useLoad<GroupsPage>(`/api/groups?page=${pageNo}`);
  const [open, setOpen] = useState<Open>(null);
  const [draft, setDraft] = useState<GroupSettings>({});
  const [flash, setFlash] = useState<string | null>(null);

  if (error && !data) return <Failed message={error} retry={reload} />;
  if (!data) return <Loading />;
  const pages = Math.max(1, Math.ceil(data.total / data.page_size));
  const done = (message: string) => {
    setOpen(null);
    setFlash(message);
    reload();
  };
  const editSettings = (g: GroupRow) => {
    setDraft({ ...(g.settings as GroupSettings) });
    setOpen({ kind: "settings", group: g });
  };
  const numberField = (key: "daily_chars" | "max_chars", label: string, hint: string) => (
    <label className="field">
      <span>{label}</span>
      <input
        className="input num"
        inputMode="numeric"
        value={draft[key] ? String(draft[key]) : ""}
        placeholder={hint}
        onChange={(e) => setDraft({ ...draft, [key]: Number(e.target.value.replace(/\D/g, "")) || 0 })}
      />
    </label>
  );
  const name = (g: GroupRow) => g.title || `chat ${g.id}`;

  return (
    <div className="stack">
      <Flash message={flash} onClose={() => setFlash(null)} />
      <section className="panel">
        {data.groups.length === 0 ? (
          <p className="empty">Ellie hasn't been added to any group yet.</p>
        ) : (
          <>
            <div className="table-wrap">
              <table>
                <thead>
                  <tr>
                    <th scope="col">Group</th>
                    <th scope="col">Status</th>
                    <th scope="col" className="right">Read today</th>
                    <th scope="col" className="right">Transcribed today</th>
                    <th scope="col">Added by</th>
                    <th scope="col">First seen</th>
                    <th scope="col">
                      <span className="visually-hidden">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.groups.map((g) => (
                    <tr key={g.id}>
                      <td>
                        <div style={{ fontWeight: 550 }}>{g.title || "Untitled chat"}</div>
                        <div className="muted num" style={{ fontSize: 12 }}>
                          {g.id}
                        </div>
                      </td>
                      <td>{status(g)}</td>
                      <td className="right num">{n(g.today_chars)}</td>
                      <td className="right num">{minutes(g.today_secs)}</td>
                      <td className="num">{g.added_by ? <a href={`#/users/${g.added_by}`}>{g.added_by}</a> : <span className="muted">Unknown</span>}</td>
                      <td className="muted">{shortDay(g.created_at.slice(0, 10))}</td>
                      <td>
                        <span style={{ display: "flex", gap: 6 }}>
                          <button className="btn btn-small" onClick={() => editSettings(g)}>
                            Settings
                          </button>
                          {g.blocked ? (
                            <button className="btn btn-small" onClick={() => setOpen({ kind: "unblock", group: g })}>
                              Unblock
                            </button>
                          ) : (
                            <button className="btn btn-small btn-danger" onClick={() => setOpen({ kind: "block", group: g })}>
                              Block
                            </button>
                          )}
                        </span>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="pager">
              <span className="muted num">
                {n(data.total)} {data.total === 1 ? "chat" : "chats"}
              </span>
              <span style={{ display: "flex", gap: 8 }}>
                <button className="btn" disabled={pageNo <= 1} onClick={() => go(`/groups?page=${pageNo - 1}`)}>
                  Previous
                </button>
                <button className="btn" disabled={pageNo >= pages} onClick={() => go(`/groups?page=${pageNo + 1}`)}>
                  Next
                </button>
              </span>
            </div>
          </>
        )}
      </section>

      {open?.kind === "block" && (
        <ActionDialog<{ left: boolean }>
          title={`Block ${name(open.group)}`}
          summary="Ellie leaves the group now and refuses to be added back until you unblock it. Members can still use Ellie in their private chat."
          confirmLabel="Block group"
          danger
          path={`/api/groups/${open.group.id}/block`}
          onClose={() => setOpen(null)}
          onDone={(out) => done(`${name(open.group)} is blocked${out.left ? " and Ellie has left it" : ""}.`)}
        />
      )}
      {open?.kind === "unblock" && (
        <ActionDialog
          title={`Unblock ${name(open.group)}`}
          summary="Its admins can add Ellie back. Ellie doesn't rejoin by itself."
          confirmLabel="Unblock group"
          path={`/api/groups/${open.group.id}/unblock`}
          onClose={() => setOpen(null)}
          onDone={() => done(`${name(open.group)} is unblocked.`)}
        />
      )}
      {open?.kind === "settings" && (
        <ActionDialog
          title={`Settings for ${name(open.group)}`}
          summary="These are the same settings the group's own admins can change. Limits can only lower the studio's, never raise them."
          confirmLabel="Save settings"
          path={`/api/groups/${open.group.id}/settings`}
          body={{ settings: draft }}
          onClose={() => setOpen(null)}
          onDone={() => done(`Saved settings for ${name(open.group)}.`)}
        >
          <div className="stack" style={{ gap: 8 }}>
            {switches.map((sw) => (
              <label className="check" key={sw.key}>
                <input type="checkbox" checked={!!draft[sw.key]} onChange={(e) => setDraft({ ...draft, [sw.key]: e.target.checked })} />
                {sw.label}
              </label>
            ))}
          </div>
          <div className="settings-grid">
            {numberField("daily_chars", "Characters a day for the group", "Studio limit")}
            {numberField("max_chars", "Longest text one message can read", "Studio limit")}
          </div>
        </ActionDialog>
      )}
    </div>
  );
}
