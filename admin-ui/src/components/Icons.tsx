// Small line icons drawn for the panel (24×24, stroke = currentColor).
const base = {
  viewBox: "0 0 24 24",
  fill: "none",
  stroke: "currentColor",
  strokeWidth: 1.8,
  strokeLinecap: "round" as const,
  strokeLinejoin: "round" as const,
  "aria-hidden": true,
};

export const IconOverview = () => (
  <svg {...base}>
    <path d="M4 12h2M8 8v8M12 5v14M16 9v6M20 11v2" />
  </svg>
);
export const IconUsers = () => (
  <svg {...base}>
    <circle cx="9" cy="8" r="3.2" />
    <path d="M3.5 19c.8-3.2 3-5 5.5-5s4.7 1.8 5.5 5M16 5.5a3 3 0 0 1 0 5.6M17.5 14c1.6.6 2.6 2.4 3 5" />
  </svg>
);
export const IconGroups = () => (
  <svg {...base}>
    <path d="M4 6.5A2.5 2.5 0 0 1 6.5 4h11A2.5 2.5 0 0 1 20 6.5v7a2.5 2.5 0 0 1-2.5 2.5H10l-4 4v-4h0A2 2 0 0 1 4 14z" />
  </svg>
);
export const IconVoices = () => (
  <svg {...base}>
    <rect x="9" y="3.5" width="6" height="10.5" rx="3" />
    <path d="M6 11.5a6 6 0 0 0 12 0M12 17.5V20.5" />
  </svg>
);
export const IconAudit = () => (
  <svg {...base}>
    <path d="M7 4h8l3 3v13H7z" />
    <path d="M10 11h5M10 15h5M10 7.5h2" />
  </svg>
);
export const IconSessions = () => (
  <svg {...base}>
    <rect x="3.5" y="5" width="17" height="11" rx="1.5" />
    <path d="M9 20h6M12 16v4" />
  </svg>
);
export const Logo = () => (
  <svg viewBox="0 0 32 32" width="30" height="30" aria-hidden="true">
    <rect width="32" height="32" rx="8" fill="var(--accent)" />
    <g fill="var(--accent-ink)">
      <rect x="7" y="13" width="3" height="6" rx="1.5" />
      <rect x="12" y="9" width="3" height="14" rx="1.5" />
      <rect x="17" y="6" width="3" height="20" rx="1.5" />
      <rect x="22" y="11" width="3" height="10" rx="1.5" />
    </g>
  </svg>
);
