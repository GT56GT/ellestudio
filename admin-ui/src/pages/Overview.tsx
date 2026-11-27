import type { Overview as OverviewData } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { Meter } from "../components/Meter";
import { Wave } from "../components/Wave";
import { minutes, n, until } from "../format";

function capText(v: number, unit: string) {
  return v > 0 ? `${n(v)} ${unit}` : `no ${unit.endsWith("transcribed") ? "transcription" : "character"} cap`;
}

export function Overview() {
  const { data, error, reload } = useLoad<OverviewData>("/api/overview", 20_000);
  if (error && !data) return <Failed message={error} retry={reload} />;
  if (!data) return <Loading rows={5} />;

  const bot = data.bot;
  const cap = bot?.limits.studio_chars ?? 0;
  const week = data.days.slice(-7);
  const sum = (k: keyof OverviewData["days"][number]) => week.reduce((t, d) => t + (d[k] as number), 0);
  const newWeek = sum("new");

  return (
    <div className="stack">
      <section className="panel meter-block" aria-labelledby="studio-today">
        <h2 id="studio-today" className="visually-hidden">
          Studio today
        </h2>
        <div className="meter-figure">
          <strong className="num">{n(data.studio_chars)}</strong>
          <span>{cap > 0 ? `of ${n(cap)} characters everyone may use today` : "characters read today, with no studio-wide cap"}</span>
        </div>
        {cap > 0 && <Meter used={data.studio_chars} cap={cap} unit="characters" />}
        <div className="meter-facts">
          <div className="fact">
            <b className="num">
              {bot ? `${bot.running} of ${bot.slots}` : "None"}
            </b>
            <span>Voice jobs running</span>
          </div>
          <div className="fact">
            <b className="num">{bot ? n(bot.queued) : "None"}</b>
            <span>Waiting in line{bot && bot.queue_limit > 0 ? `, up to ${n(bot.queue_limit)}` : ""}</span>
          </div>
          <div className="fact">
            <b className="num">
              {minutes(data.studio_secs)}
              {bot && bot.limits.studio_audio_secs > 0 ? ` of ${minutes(bot.limits.studio_audio_secs)}` : ""}
            </b>
            <span>Transcribed today</span>
          </div>
          <div className="fact">
            <b className="num">
              {n(data.groups_chars)}
              {bot && bot.limits.groups_total_chars > 0 ? ` of ${n(bot.limits.groups_total_chars)}` : ""}
            </b>
            <span>Characters read in groups</span>
          </div>
          {bot?.paused_until && (
            <div className="fact">
              <b style={{ color: "var(--warn)" }}>Paused for {until(bot.paused_until)}</b>
              <span>Cartesia kept failing</span>
            </div>
          )}
        </div>
      </section>

      <section className="panel figures" aria-label="The last 7 days">
        <div>
          <b className="num">{n(data.week_active)}</b>
          <span>People used Ellie this week</span>
        </div>
        <div>
          <b className="num">{n(sum("readings"))}</b>
          <span>Readings this week</span>
        </div>
        <div>
          <b className="num">{n(newWeek)}</b>
          <span>New people this week</span>
        </div>
        <div>
          <b className="num">{n(sum("limit_users"))}</b>
          <span>Times someone hit their daily limit</span>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>Last 30 days</h2>
          <span className="muted">UTC days</span>
        </div>
        <div className="panel-body">
          <Wave
            points={data.days.map((d) => ({ day: d.day, up: d.readings, down: d.active }))}
            upLabel="Readings"
            downLabel="Active people"
          />
        </div>
      </section>

      <div className="grid-2">
        <section className="panel">
          <div className="panel-head">
            <h2>Where new people came from</h2>
            <span className="muted">7 days</span>
          </div>
          <div className="panel-body sources">
            {[
              { label: "Saw Ellie in a group", value: sum("from_groups") },
              { label: "Invited by a friend", value: sum("from_invites") },
              { label: "Found Ellie directly", value: Math.max(0, newWeek - sum("from_groups") - sum("from_invites")) },
            ].map((s) => (
              <div className="source-row" key={s.label}>
                <span>{s.label}</span>
                <span className="num">{n(s.value)}</span>
                <div className="bar" aria-hidden="true">
                  <i style={{ width: `${newWeek ? (s.value / newWeek) * 100 : 0}%` }} />
                </div>
              </div>
            ))}
            <p className="muted" style={{ margin: 0, fontSize: 14 }}>
              {n(data.users)} people have used Ellie, {n(data.joined)} of them in the private chat. Ellie is in {n(data.groups)}{" "}
              {data.groups === 1 ? "group" : "groups"}.
            </p>
          </div>
        </section>
        <section className="panel">
          <div className="panel-head">
            <h2>Daily limits</h2>
            <span className="muted">Reset at midnight UTC</span>
          </div>
          <div className="panel-body">
            {bot ? (
              <dl className="facts-grid" style={{ gridTemplateColumns: "1fr" }}>
                {[
                  ["Each person", `${capText(bot.limits.user_chars, "characters")}, ${capText(bot.limits.user_audio_secs / 60, "min transcribed")}`],
                  ["Each group", capText(bot.limits.group_chars, "characters")],
                  ["All groups together", capText(bot.limits.groups_total_chars, "characters")],
                  ["Everyone together", `${capText(bot.limits.studio_chars, "characters")}, ${capText(bot.limits.studio_audio_secs / 60, "min transcribed")}`],
                ].map(([k, v]) => (
                  <div key={k}>
                    <dt>{k}</dt>
                    <dd className="num" style={{ fontSize: 15 }}>
                      {v}
                    </dd>
                  </div>
                ))}
              </dl>
            ) : (
              <p className="muted" style={{ margin: 0 }}>
                Ellie hasn't reported its limits yet. They appear once the latest bot version is running.
              </p>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
