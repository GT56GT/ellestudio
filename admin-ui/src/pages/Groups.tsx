import type { GroupRow } from "../types";
import { useLoad } from "../components/useLoad";
import { Failed, Loading } from "../components/States";
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

export function Groups({ route }: { route: Route }) {
  const pageNo = Number(route.params.get("page") ?? "1") || 1;
  const { data, error, reload } = useLoad<GroupsPage>(`/api/groups?page=${pageNo}`);
  if (error && !data) return <Failed message={error} retry={reload} />;
  if (!data) return <Loading />;
  const pages = Math.max(1, Math.ceil(data.total / data.page_size));

  return (
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
  );
}
