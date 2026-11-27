import { useState, type FormEvent } from "react";
import type { UserRow } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
import { ago, n } from "../format";
import { go, type Route } from "../router";

interface UsersPage {
  users: UserRow[];
  total: number;
  page: number;
  page_size: number;
}

const sorts = [
  { value: "recent", label: "Recently active" },
  { value: "readings", label: "Most readings" },
  { value: "bonus", label: "Most bonus characters" },
];

export function Users({ route }: { route: Route }) {
  const q = route.params.get("q") ?? "";
  const sort = route.params.get("sort") ?? "recent";
  const pageNo = Number(route.params.get("page") ?? "1") || 1;
  const [draft, setDraft] = useState(q);
  const qs = new URLSearchParams({ sort, page: String(pageNo), ...(q ? { q } : {}) });
  const { data, error, reload } = useLoad<UsersPage>(`/api/users?${qs}`);

  const nav = (next: Record<string, string>) => {
    const p = new URLSearchParams({ sort, ...(q ? { q } : {}), ...next });
    go(`/users?${p}`);
  };
  const search = (e: FormEvent) => {
    e.preventDefault();
    nav({ q: draft.trim(), page: "1" });
  };
  const pages = data ? Math.max(1, Math.ceil(data.total / data.page_size)) : 1;

  return (
    <section className="panel">
      <form className="toolbar" onSubmit={search} role="search">
        <label className="visually-hidden" htmlFor="user-search">
          Telegram user ID
        </label>
        <input
          id="user-search"
          className="input num"
          inputMode="numeric"
          placeholder="Find by Telegram user ID"
          value={draft}
          onChange={(e) => setDraft(e.target.value)}
        />
        <button className="btn" type="submit">
          Find
        </button>
        {q && (
          <button className="link-btn" type="button" onClick={() => (setDraft(""), nav({ q: "", page: "1" }))}>
            Show everyone
          </button>
        )}
        <label className="visually-hidden" htmlFor="user-sort">
          Sort by
        </label>
        <select id="user-sort" className="select" value={sort} onChange={(e) => nav({ sort: e.target.value, page: "1" })} style={{ marginLeft: "auto" }}>
          {sorts.map((s) => (
            <option key={s.value} value={s.value}>
              {s.label}
            </option>
          ))}
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
      ) : data.users.length === 0 ? (
        <p className="empty">{q ? `Nobody with ID ${q} has used Ellie.` : "Nobody has used Ellie yet."}</p>
      ) : (
        <>
          <div className="table-wrap">
            <table>
              <thead>
                <tr>
                  <th scope="col">User ID</th>
                  <th scope="col">Voice</th>
                  <th scope="col" className="right">Today</th>
                  <th scope="col" className="right">Readings</th>
                  <th scope="col" className="right">Bonus</th>
                  <th scope="col" className="right">Invited</th>
                  <th scope="col">Last active</th>
                </tr>
              </thead>
              <tbody>
                {data.users.map((u) => (
                  <tr key={u.id} data-link onClick={() => go(`/users/${u.id}`)}>
                    <td className="num">
                      <a href={`#/users/${u.id}`} onClick={(e) => e.stopPropagation()}>
                        {u.id}
                      </a>
                      {!u.joined && <span className="pill" style={{ marginLeft: 8 }}>Groups only</span>}
                    </td>
                    <td>{u.voice || <span className="muted">Default</span>}</td>
                    <td className="right num">{n(u.today_chars)}</td>
                    <td className="right num">{n(u.readings)}</td>
                    <td className="right num">{n(u.bonus)}</td>
                    <td className="right num">{n(u.referrals)}</td>
                    <td className="muted">{ago(u.updated_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="pager">
            <span className="muted num">
              {n(data.total)} {data.total === 1 ? "person" : "people"}
            </span>
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
  );
}
