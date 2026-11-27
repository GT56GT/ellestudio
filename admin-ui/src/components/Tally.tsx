import type { BotStatus } from "../types";
import { useLoad } from "./useLoad";
import { ago, until } from "../format";

// Tally is the studio's on-air light: live, paused or offline.
export function Tally() {
  const { data } = useLoad<{ bot: BotStatus | null; bot_online: boolean }>("/api/status", 15_000);
  if (!data) return <span className="tally" data-state="unknown"><span className="lamp" />Checking Ellie</span>;
  const { bot, bot_online } = data;
  if (!bot) {
    return (
      <span className="tally" data-state="offline" title="The bot hasn't reported yet. Deploy the latest version of the bot.">
        <span className="lamp" />
        No word from Ellie yet
      </span>
    );
  }
  if (!bot_online) {
    return (
      <span className="tally" data-state="offline">
        <span className="lamp" />
        Ellie is offline <span className="muted">last seen {ago(bot.at)}</span>
      </span>
    );
  }
  if (bot.paused_until) {
    return (
      <span className="tally" data-state="paused">
        <span className="lamp" />
        Voices paused <span className="muted">for {until(bot.paused_until)}</span>
      </span>
    );
  }
  return (
    <span className="tally" data-state="live" title={`Instance ${bot.instance}, running since ${new Date(bot.started_at).toLocaleString()}`}>
      <span className="lamp" />
      Ellie is live {bot.version && <span className="muted num">version {bot.version}</span>}
    </span>
  );
}
