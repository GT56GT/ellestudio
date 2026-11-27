export function Failed({ message, retry }: { message: string; retry?: () => void }) {
  return (
    <div className="notice" data-tone="hot" role="alert">
      {message}{" "}
      {retry && (
        <button type="button" className="link-btn" onClick={retry}>
          Try again
        </button>
      )}
    </div>
  );
}

export function Loading({ rows = 3 }: { rows?: number }) {
  return (
    <div className="stack" aria-busy="true" aria-label="Loading">
      {Array.from({ length: rows }, (_, i) => (
        <div key={i} className="skeleton" style={{ width: `${90 - i * 15}%` }} />
      ))}
    </div>
  );
}
