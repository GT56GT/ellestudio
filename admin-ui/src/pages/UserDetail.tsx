import type { UserDetail as Detail } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { Wave } from "../components/Wave";
import { ago, n, shortDay } from "../format";

export function UserDetail({ id }: { id: string }) {
  const { data, error, reload } = useLoad<Detail>(`/api/users/${encodeURIComponent(id)}`);
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
      {error && !data ? (
        <Failed message={error} retry={reload} />
      ) : !data ? (
        <Loading rows={4} />
      ) : (
        <>
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
                    {k === "Invited by" && data.referred_by ? <a href={`#/users/${data.referred_by}`}>{v}</a> : v}
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
                <Wave
                  points={data.activity.map((a) => ({ day: a.day, up: a.readings, down: a.chars }))}
                  upLabel="Readings"
                  downLabel="Characters"
                />
              )}
            </div>
          </section>
        </>
      )}
    </div>
  );
}
