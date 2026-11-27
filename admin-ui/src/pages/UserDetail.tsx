import { useState } from "react";
import type { UserDetail as Detail } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { ActionDialog, Flash } from "../components/ActionDialog";
import { Wave } from "../components/Wave";
import { ago, dateTime, n, shortDay } from "../format";

type Dialog = "bonus" | "reset" | "ban" | "unban" | null;

const banLengths = [
  { days: 1, label: "1 day" },
  { days: 7, label: "7 days" },
  { days: 30, label: "30 days" },
  { days: 0, label: "Until I lift it" },
];

export function UserDetail({ id, adminIds }: { id: string; adminIds: number[] }) {
  const { data, error, reload } = useLoad<Detail>(`/api/users/${encodeURIComponent(id)}`);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [flash, setFlash] = useState<string | null>(null);
  const [amount, setAmount] = useState("500");
  const [notify, setNotify] = useState(true);
  const [days, setDays] = useState(7);

  const done = (message: string) => {
    setDialog(null);
    setFlash(message);
    reload();
  };
  const told = (out: { notified?: boolean; notify_error?: string }) =>
    out.notified ? " They were told in Telegram." : out.notify_error ? ` ${out.notify_error}` : "";
  const amountN = Number(amount);
  const isAdmin = adminIds.includes(Number(id));

  return (
    <div className="stack">
      <div>
        <a className="back" href="#/users">
          ← All users
        </a>
        <h1 className="page-title num">User {id}</h1>
        {data && (
          <p className="page-lede">
            {data.joined ? `Uses the private chat${data.joined_at ? ` since ${shortDay(data.joined_at)}` : ""}` : "Only uses Ellie in groups"}.
            Last active {ago(data.updated_at)}.
          </p>
        )}
      </div>
      <Flash message={flash} onClose={() => setFlash(null)} />
      {error && !data ? (
        <Failed message={error} retry={reload} />
      ) : !data ? (
        <Loading rows={4} />
      ) : (
        <>
          {data.banned && data.ban && (
            <div className="notice banner-ban" role="status">
              <b>Banned</b> {data.ban.until ? `until ${dateTime(data.ban.until)}` : "until an admin lifts it"}.{" "}
              {data.ban.reason && <span className="muted">Reason: {data.ban.reason}</span>}
            </div>
          )}

          <section className="panel panel-body">
            <div className="actions-row">
              <button className="btn" onClick={() => setDialog("bonus")}>
                Give bonus characters
              </button>
              <button className="btn" onClick={() => setDialog("reset")} disabled={!data.today_chars && !data.today_secs}>
                Reset today's allowance
              </button>
              {data.banned ? (
                <button className="btn" onClick={() => setDialog("unban")}>
                  Lift ban
                </button>
              ) : (
                <button className="btn btn-danger" onClick={() => setDialog("ban")} disabled={isAdmin} title={isAdmin ? "Admins can't be banned" : undefined}>
                  Ban
                </button>
              )}
            </div>
          </section>

          <section className="panel panel-body">
            <dl className="facts-grid">
              {[
                ["Read today", `${n(data.today_chars)} characters`],
                ["Transcribed today", `${Math.round(data.today_secs / 60)} min`],
                ["Readings, all time", n(data.readings)],
                ["Characters, all time", n(data.spoken_chars)],
                ["Bonus characters", n(data.bonus)],
                ["Friends invited", n(data.referrals)],
                ["Invited by", data.referred_by ? String(data.referred_by) : "Nobody"],
                ["Voice", data.voice || "Default"],
                ["Language", data.language || "Automatic"],
                ["Emotion", data.emotion || "Automatic"],
                ["Voices made", n(data.voices)],
                ["Dictionaries made", n(data.dicts)],
              ].map(([k, v]) => (
                <div key={k}>
                  <dt>{k}</dt>
                  <dd className="num">
                    {k === "Invited by" && data.referred_by ? (
                      <a href={`#/users/${data.referred_by}`}>{v}</a>
                    ) : k === "Voices made" && data.voices + data.dicts > 0 ? (
                      <a href={`#/voices?owner=${data.id}`}>{v}</a>
                    ) : (
                      v
                    )}
                  </dd>
                </div>
              ))}
            </dl>
          </section>
          <section className="panel">
            <div className="panel-head">
              <h2>Readings, last 30 days</h2>
            </div>
            <div className="panel-body">
              {data.activity.length === 0 ? (
                <p className="empty">No readings in the last 30 days.</p>
              ) : (
                <Wave points={data.activity.map((a) => ({ day: a.day, up: a.readings, down: a.chars }))} upLabel="Readings" downLabel="Characters" />
              )}
            </div>
          </section>

          {dialog === "bonus" && (
            <ActionDialog<{ after: number; notified?: boolean; notify_error?: string }>
              title="Give bonus characters"
              summary={
                <>
                  User {id} has <b>{n(data.bonus)}</b> bonus characters. They're spent once the daily allowance runs out, and never expire.
                </>
              }
              confirmLabel={Number.isInteger(amountN) && amountN > 0 ? `Give ${n(amountN)} characters` : "Give characters"}
              path={`/api/users/${id}/bonus`}
              body={{ amount: amountN, notify }}
              blocked={!Number.isInteger(amountN) || amountN < 1 || amountN > 100000 ? "Enter 1 to 100,000" : null}
              onClose={() => setDialog(null)}
              onDone={(out) => done(`Gave ${n(amountN)} bonus characters; they now have ${n(out.after)}.${told(out)}`)}
            >
              <label className="field">
                <span>Characters</span>
                <input className="input num" inputMode="numeric" value={amount} onChange={(e) => setAmount(e.target.value.replace(/\D/g, ""))} />
              </label>
              <label className="check">
                <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} />
                Tell them in Telegram
              </label>
            </ActionDialog>
          )}

          {dialog === "reset" && (
            <ActionDialog
              title="Reset today's allowance"
              summary={
                <>
                  User {id} has used <b>{n(data.today_chars)}</b> characters and <b>{Math.round(data.today_secs / 60)} min</b> today. This sets
                  both back to zero until midnight UTC. Bonus characters aren't touched.
                </>
              }
              confirmLabel="Reset to zero"
              path={`/api/users/${id}/reset-today`}
              onClose={() => setDialog(null)}
              onDone={() => done("Today's allowance is back to full.")}
            />
          )}

          {dialog === "ban" && (
            <ActionDialog
              title={`Ban user ${id}`}
              summary="Ellie will ignore them everywhere: the private chat, groups, guest replies and buttons. They get one short message saying their access is paused. It takes effect within 30 seconds."
              confirmLabel={days ? `Ban for ${banLengths.find((b) => b.days === days)?.label}` : "Ban until I lift it"}
              danger
              path={`/api/users/${id}/ban`}
              body={{ days }}
              onClose={() => setDialog(null)}
              onDone={() => done(`User ${id} is banned ${days ? `for ${banLengths.find((b) => b.days === days)?.label}` : "until you lift it"}.`)}
            >
              <fieldset className="choices" style={{ border: 0, padding: 0, margin: 0 }}>
                <legend className="visually-hidden">How long</legend>
                {banLengths.map((b) => (
                  <label key={b.days}>
                    <input type="radio" name="days" checked={days === b.days} onChange={() => setDays(b.days)} />
                    {b.label}
                  </label>
                ))}
              </fieldset>
            </ActionDialog>
          )}

          {dialog === "unban" && (
            <ActionDialog<{ notified?: boolean; notify_error?: string }>
              title={`Lift the ban on user ${id}`}
              summary="They can use Ellie again within 30 seconds."
              confirmLabel="Lift ban"
              path={`/api/users/${id}/unban`}
              body={{ notify }}
              onClose={() => setDialog(null)}
              onDone={(out) => done(`The ban on user ${id} is lifted.${told(out)}`)}
            >
              <label className="check">
                <input type="checkbox" checked={notify} onChange={(e) => setNotify(e.target.checked)} />
                Tell them in Telegram
              </label>
            </ActionDialog>
          )}
        </>
      )}
    </div>
  );
}
