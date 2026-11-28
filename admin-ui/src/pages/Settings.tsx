import { useEffect, useState } from "react";
import type { OverrideKey, Overrides, StatusLimits } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { ActionDialog, Flash } from "../components/ActionDialog";
import { n } from "../format";

interface SettingsData {
  overrides: Overrides;
  defaults: StatusLimits | null;
  limits: StatusLimits | null;
  bot_online: boolean;
}

// Each field: the override key, the label, the default's key in the
// heartbeat, and the unit shown (minutes are stored as seconds).
interface Field {
  key: OverrideKey;
  label: string;
  from: keyof StatusLimits;
  unit: "characters" | "min" | "seconds" | "count";
}

const groups: { title: string; note: string; fields: Field[] }[] = [
  {
    title: "Each person, per day",
    note: "Admins are never limited.",
    fields: [
      { key: "daily_chars", label: "Characters read aloud", from: "user_chars", unit: "characters" },
      { key: "daily_audio_secs", label: "Transcription", from: "user_audio_secs", unit: "min" },
      { key: "daily_creations", label: "New voices", from: "user_creations", unit: "count" },
      { key: "daily_dicts", label: "New dictionaries", from: "user_dicts", unit: "count" },
      { key: "max_tts_chars", label: "Longest text in one message", from: "max_tts_chars", unit: "characters" },
    ],
  },
  {
    title: "Everyone together, per day",
    note: "Protects your Cartesia credits. 0 means no cap.",
    fields: [
      { key: "studio_daily_chars", label: "Characters", from: "studio_chars", unit: "characters" },
      { key: "studio_daily_audio_secs", label: "Transcription", from: "studio_audio_secs", unit: "min" },
    ],
  },
  {
    title: "Groups",
    note: "Group admins can lower their own group's limits, never raise them.",
    fields: [
      { key: "group_chars", label: "Characters per group, per day", from: "group_chars", unit: "characters" },
      { key: "group_secs", label: "Transcription per group, per day", from: "group_secs", unit: "min" },
      { key: "groups_total_chars", label: "Characters, all groups together", from: "groups_total_chars", unit: "characters" },
      { key: "groups_total_secs", label: "Transcription, all groups together", from: "groups_total_secs", unit: "min" },
      { key: "group_cooldown_secs", label: "Wait between one person's requests", from: "group_cooldown_secs", unit: "seconds" },
    ],
  },
  {
    title: "Invites",
    note: "Bonus characters for a new user and the friend who invited them. 0 turns invites off.",
    fields: [
      { key: "referral_bonus", label: "Bonus characters each", from: "referral_bonus", unit: "characters" },
      { key: "referral_daily_cap", label: "Rewards an inviter can earn a day", from: "referral_daily_cap", unit: "count" },
    ],
  },
];

const allFields = groups.flatMap((g) => g.fields);
const toShown = (f: Field, stored: number) => (f.unit === "min" ? Math.round(stored / 60) : stored);
const toStored = (f: Field, shown: number) => (f.unit === "min" ? shown * 60 : shown);
const unitLabel = (f: Field) => (f.unit === "count" ? "" : f.unit === "min" ? "min" : f.unit === "seconds" ? "s" : "chars");

export function Settings() {
  const { data, error, reload } = useLoad<SettingsData>("/api/settings");
  // The form: a shown value per overridden key; "" means "use the default".
  const [form, setForm] = useState<Record<string, string>>({});
  const [dialog, setDialog] = useState<"save" | "pause" | "resume" | null>(null);
  const [flash, setFlash] = useState<string | null>(null);

  useEffect(() => {
    if (!data) return;
    const next: Record<string, string> = {};
    for (const f of allFields) {
      const v = data.overrides[f.key];
      next[f.key] = v === undefined ? "" : String(toShown(f, v));
    }
    setForm(next);
  }, [data]);

  if (error && !data) return <Failed message={error} retry={reload} />;
  if (!data) return <Loading rows={6} />;

  const overrides: Overrides = {};
  const changes: string[] = [];
  for (const f of allFields) {
    const raw = form[f.key] ?? "";
    const before = data.overrides[f.key];
    if (raw !== "") overrides[f.key] = toStored(f, Number(raw));
    const after = overrides[f.key];
    if (before !== after) {
      const def = data.defaults ? toShown(f, data.defaults[f.from]) : null;
      changes.push(`${f.label}: ${after === undefined ? `back to the default${def !== null ? ` (${n(def)})` : ""}` : n(toShown(f, after))}`);
    }
  }
  const paused = !!data.overrides.paused;
  const done = (message: string) => {
    setDialog(null);
    setFlash(message);
    reload();
  };

  return (
    <div className="stack">
      <Flash message={flash} onClose={() => setFlash(null)} />

      <section className="panel panel-body" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 16, flexWrap: "wrap" }}>
        <div>
          <h2 style={{ margin: 0, fontSize: 16 }}>{paused ? "Voice work is paused" : "Emergency pause"}</h2>
          <p className="muted" style={{ margin: "4px 0 0", fontSize: 14 }}>
            {paused
              ? "Nobody can start a reading, transcription or new voice. People see “Ellie is taking a short break” and aren't charged."
              : "Stops all new voice work at once, for example during a Cartesia incident. Menus and settings keep working."}
          </p>
        </div>
        {paused ? (
          <button className="btn btn-primary" onClick={() => setDialog("resume")}>
            Resume voice work
          </button>
        ) : (
          <button className="btn btn-danger-solid" onClick={() => setDialog("pause")}>
            Pause voice work
          </button>
        )}
      </section>

      {!data.defaults && (
        <div className="notice">Ellie hasn't reported its defaults yet, so empty fields show no default. They fill in once the latest bot is running.</div>
      )}

      {groups.map((g) => (
        <section className="panel" key={g.title}>
          <div className="panel-head">
            <h2>{g.title}</h2>
            <span className="muted" style={{ fontSize: 14 }}>
              {g.note}
            </span>
          </div>
          <div className="panel-body settings-grid">
            {g.fields.map((f) => {
              const def = data.defaults ? toShown(f, data.defaults[f.from]) : null;
              const changed = (form[f.key] ?? "") !== "";
              return (
                <label className="field" key={f.key}>
                  <span>
                    {f.label} {unitLabel(f) && <span className="muted">({unitLabel(f)})</span>}
                  </span>
                  <span style={{ display: "flex", gap: 8, alignItems: "center" }}>
                    <input
                      className="input num"
                      inputMode="numeric"
                      value={form[f.key] ?? ""}
                      placeholder={def !== null ? `${n(def)} (default)` : "Default"}
                      onChange={(e) => setForm({ ...form, [f.key]: e.target.value.replace(/\D/g, "") })}
                    />
                    {changed && (
                      <button type="button" className="link-btn" onClick={() => setForm({ ...form, [f.key]: "" })}>
                        Default
                      </button>
                    )}
                  </span>
                </label>
              );
            })}
          </div>
        </section>
      ))}

      <div style={{ display: "flex", justifyContent: "flex-end", gap: 12, alignItems: "center" }}>
        <span className="muted" style={{ fontSize: 14 }}>
          {changes.length ? `${changes.length} unsaved ${changes.length === 1 ? "change" : "changes"}` : "Empty fields use the server's defaults."}
        </span>
        <button className="btn btn-primary" disabled={!changes.length} onClick={() => setDialog("save")}>
          Save limits
        </button>
      </div>

      {dialog === "save" && (
        <ActionDialog
          title="Save limits"
          summary={
            <>
              Ellie switches to these within 30 seconds, without a redeploy:
              <ul style={{ margin: "8px 0 0", paddingLeft: 18 }}>
                {changes.map((c) => (
                  <li key={c}>{c}</li>
                ))}
              </ul>
            </>
          }
          confirmLabel="Save limits"
          path="/api/settings"
          body={{ overrides }}
          onClose={() => setDialog(null)}
          onDone={() => done("Saved. Ellie uses the new limits within 30 seconds.")}
        />
      )}
      {dialog === "pause" && (
        <ActionDialog
          title="Pause voice work"
          summary="Within 30 seconds, nobody can start a reading, transcription or new voice until you resume. Jobs already running finish."
          confirmLabel="Pause voice work"
          danger
          path="/api/pause"
          body={{ paused: true }}
          onClose={() => setDialog(null)}
          onDone={() => done("Voice work pauses within 30 seconds.")}
        />
      )}
      {dialog === "resume" && (
        <ActionDialog
          title="Resume voice work"
          summary="Ellie starts taking readings again within 30 seconds."
          confirmLabel="Resume voice work"
          path="/api/pause"
          body={{ paused: false }}
          onClose={() => setDialog(null)}
          onDone={() => done("Voice work resumes within 30 seconds.")}
        />
      )}
    </div>
  );
}
