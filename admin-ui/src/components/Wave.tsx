import { n, shortDay } from "../format";

export interface WavePoint {
  day: string;
  up: number;
  down: number;
}

// Wave draws days as a mirrored waveform: one series above the axis and one
// below, so two related counts read as a single shape.
export function Wave({ points, upLabel, downLabel }: { points: WavePoint[]; upLabel: string; downLabel: string }) {
  const W = 960;
  const H = 200;
  const mid = H / 2;
  const gap = 3;
  const bw = points.length ? W / points.length - gap : 0;
  const peakUp = Math.max(1, ...points.map((p) => p.up));
  const peakDown = Math.max(1, ...points.map((p) => p.down));
  const half = mid - 14;
  const labelEvery = Math.ceil(points.length / 6);

  return (
    <figure style={{ margin: 0 }}>
      <svg className="wave" viewBox={`0 0 ${W} ${H + 22}`} role="img" aria-label={`${upLabel} and ${downLabel} per day`}>
        <line className="axis" x1="0" x2={W} y1={mid} y2={mid} />
        {points.map((p, i) => {
          const x = i * (bw + gap);
          const hu = Math.max(p.up ? 2 : 0, (p.up / peakUp) * half);
          const hd = Math.max(p.down ? 2 : 0, (p.down / peakDown) * half);
          return (
            <g key={p.day}>
              <title>{`${shortDay(p.day)}: ${n(p.up)} ${upLabel.toLowerCase()}, ${n(p.down)} ${downLabel.toLowerCase()}`}</title>
              <rect className="up" x={x} y={mid - hu - 1} width={bw} height={hu} rx={Math.min(2, bw / 2)} />
              <rect className="down" x={x} y={mid + 1} width={bw} height={hd} rx={Math.min(2, bw / 2)} />
              {i % labelEvery === 0 && (
                <text x={x} y={H + 18}>
                  {shortDay(p.day)}
                </text>
              )}
            </g>
          );
        })}
      </svg>
      <figcaption className="legend">
        <span>
          <i style={{ background: "var(--accent)" }} />
          {upLabel}
        </span>
        <span>
          <i style={{ background: "color-mix(in srgb, var(--accent) 38%, var(--panel))" }} />
          {downLabel}
        </span>
      </figcaption>
    </figure>
  );
}
