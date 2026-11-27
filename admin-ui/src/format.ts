const num = new Intl.NumberFormat("en");

export const n = (v: number) => num.format(v);

export function minutes(secs: number) {
  const m = secs / 60;
  return `${Number.isInteger(m) ? m : m.toFixed(1)} min`;
}

export function ago(iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((now - new Date(iso).getTime()) / 1000));
  if (s < 45) return "just now";
  const m = Math.round(s / 60);
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 36) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}

export function until(iso: string, now = Date.now()) {
  const s = Math.max(0, Math.round((new Date(iso).getTime() - now) / 1000));
  return s < 60 ? `${s} s` : `${Math.ceil(s / 60)} min`;
}

export function dateTime(iso: string) {
  return new Date(iso).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

export function shortDay(day: string) {
  return new Date(`${day}T00:00:00Z`).toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" });
}

// A browser name from a user agent, for the sessions list.
export function device(ua: string) {
  if (!ua) return "Unknown device";
  if (/Telegram/i.test(ua)) return "Telegram app";
  const os = /iPhone|iPad/.test(ua) ? "iOS" : /Android/.test(ua) ? "Android" : /Mac OS X/.test(ua) ? "macOS" : /Windows/.test(ua) ? "Windows" : /Linux/.test(ua) ? "Linux" : "";
  const browser = /Edg\//.test(ua) ? "Edge" : /Firefox\//.test(ua) ? "Firefox" : /Chrome\//.test(ua) ? "Chrome" : /Safari\//.test(ua) ? "Safari" : "Browser";
  return os ? `${browser} on ${os}` : browser;
}
