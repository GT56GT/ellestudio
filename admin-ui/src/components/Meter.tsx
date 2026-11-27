import { n } from "../format";

const SEGMENTS = 40;

// Meter is the studio's daily level meter: segments fill green, then amber
// past 70% and red past 90%, like a mixing desk's VU meter.
export function Meter({ used, cap, unit }: { used: number; cap: number; unit: string }) {
  const share = cap > 0 ? Math.min(1, used / cap) : 0;
  const lit = cap > 0 ? Math.round(share * SEGMENTS) : 0;
  return (
    <>
      <div
        className="meter"
        role="meter"
        aria-valuemin={0}
        aria-valuemax={cap}
        aria-valuenow={used}
        aria-label={`${n(used)} of ${n(cap)} ${unit} used today`}
      >
        {Array.from({ length: SEGMENTS }, (_, i) => {
          const at = (i + 1) / SEGMENTS;
          const tone = at > 0.9 ? "hot" : at > 0.7 ? "warn" : "ok";
          return <i key={i} data-on={i < lit ? tone : undefined} style={{ ["--i" as string]: i }} />;
        })}
      </div>
      <div className="meter-scale num" aria-hidden="true">
        <span>0</span>
        <span>{n(Math.round(cap / 2))}</span>
        <span>{n(cap)}</span>
      </div>
    </>
  );
}
