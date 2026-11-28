// Shapes returned by the panel's JSON API (internal/admin/store.go).

export interface BotStatus {
  instance: string;
  version: string;
  started_at: string;
  at: string;
  running: number;
  slots: number;
  queued: number;
  queue_limit: number;
  paused_until?: string;
  admin_paused?: boolean;
  limits: StatusLimits;
  defaults?: StatusLimits;
}

export interface StatusLimits {
  user_chars: number;
  user_audio_secs: number;
  user_creations: number;
  user_dicts: number;
  max_tts_chars: number;
  studio_chars: number;
  studio_audio_secs: number;
  group_chars: number;
  group_secs: number;
  groups_total_chars: number;
  groups_total_secs: number;
  group_cooldown_secs: number;
  referral_bonus: number;
  referral_daily_cap: number;
}

// Overrides an admin saved; a missing key means the server default.
export type Overrides = Partial<Record<OverrideKey, number>> & { paused?: boolean };
export type OverrideKey =
  | "daily_chars"
  | "daily_audio_secs"
  | "daily_creations"
  | "daily_dicts"
  | "max_tts_chars"
  | "studio_daily_chars"
  | "studio_daily_audio_secs"
  | "group_chars"
  | "group_secs"
  | "groups_total_chars"
  | "groups_total_secs"
  | "group_cooldown_secs"
  | "referral_bonus"
  | "referral_daily_cap";

export interface BroadcastRow {
  id: number;
  text: string;
  audience: "everyone" | "active_30d";
  created_by: number;
  status: "queued" | "sending" | "done" | "cancelled";
  total: number;
  sent: number;
  failed: number;
  created_at: string;
  started_at: string | null;
  finished_at: string | null;
}

export interface Day {
  day: string;
  active: number;
  new: number;
  from_groups: number;
  from_invites: number;
  readings: number;
  chars: number;
  limit_users: number;
  guest_invites: number;
}

export interface Overview {
  bot: BotStatus | null;
  bot_online: boolean;
  studio_chars: number;
  studio_secs: number;
  groups_chars: number;
  days: Day[];
  week_active: number;
  users: number;
  joined: number;
  groups: number;
}

export interface UserRow {
  id: number;
  joined: boolean;
  joined_at: string;
  voice: string;
  readings: number;
  spoken_chars: number;
  bonus: number;
  referrals: number;
  today_chars: number;
  today_secs: number;
  banned: boolean;
  updated_at: string;
}

export interface Ban {
  until?: string;
  reason?: string;
  by?: number;
  at: string;
}

export interface UserDetail extends UserRow {
  referred_by: number;
  ban?: Ban;
  language: string;
  model: string;
  emotion: string;
  voices: number;
  dicts: number;
  activity: { day: string; readings: number; chars: number }[];
}

export interface GroupRow {
  id: number;
  type: string;
  title: string;
  status: string;
  blocked: boolean;
  added_by: number;
  today_chars: number;
  today_secs: number;
  settings: Record<string, unknown>;
  created_at: string;
}

export interface AuditEntry {
  id: number;
  at: string;
  admin_id: number;
  action: string;
  target: string;
  detail: Record<string, unknown>;
  ip: string;
}

export interface Session {
  id: string;
  created_at: string;
  last_seen: string;
  expires_at: string;
  user_agent: string;
  ip: string;
  current: boolean;
}

export interface Me {
  admin: { id: number; name: string };
  admin_ids: number[];
  expires_at: string;
}

export interface GroupSettings {
  paused?: boolean;
  admins_only?: boolean;
  no_speak?: boolean;
  no_transcribe?: boolean;
  daily_chars?: number;
  max_chars?: number;
}

export interface VoiceRow {
  kind: "voice" | "dict";
  id: string;
  name: string;
  owner_id: number;
  created_at: string;
  job_status?: "pending" | "running" | "failed";
  job_error?: string;
}

export interface JobRow {
  id: number;
  kind: string;
  target: string;
  requested_by: number;
  status: "pending" | "running" | "done" | "failed";
  error: string;
  created_at: string;
  updated_at: string;
}
