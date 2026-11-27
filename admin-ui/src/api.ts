// The panel's API client. Inside Telegram the session token is kept in this
// module's memory (never in storage); in a browser it lives in an HttpOnly
// cookie the page can't read.

let bearer: string | null = null;

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

// Called when any request finds the session gone.
let onSignedOut: () => void = () => {};
export function whenSignedOut(fn: () => void) {
  onSignedOut = fn;
}

async function request<T>(method: "GET" | "POST", path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json" };
  if (bearer) headers.Authorization = `Bearer ${bearer}`;
  if (method === "POST") {
    headers["X-Ellie-Admin"] = "1"; // the server's CSRF check
    if (body !== undefined) headers["Content-Type"] = "application/json";
  }
  let res: Response;
  try {
    res = await fetch(path, {
      method,
      headers,
      credentials: "same-origin",
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, "Can't reach the panel. Check your connection and try again.");
  }
  if (res.status === 204) return undefined as T;
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    if (res.status === 401 && !path.startsWith("/api/auth/")) onSignedOut();
    throw new ApiError(res.status, (data as { error?: string }).error ?? `Request failed (${res.status}).`);
  }
  return data as T;
}

export const get = <T>(path: string) => request<T>("GET", path);
export const post = <T>(path: string, body?: unknown) => request<T>("POST", path, body);

// signInWithTelegramApp exchanges Mini App launch data for a session.
export async function signInWithTelegramApp(initData: string) {
  const out = await post<{ token: string; admin: { id: number; name: string } }>("/api/auth/webapp", {
    init_data: initData,
  });
  bearer = out.token;
  return out.admin;
}

export async function signOut() {
  try {
    await post("/api/auth/logout");
  } finally {
    bearer = null;
  }
}
