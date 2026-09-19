// How a run still to come is written on the workflows page: the moment
// itself, and how long there is until it.
//
// The times come from the server as RFC 3339 instants, so they carry the
// offset the scheduler will run them at; the browser shows them in its own
// zone, which on the machine serving the editor is the same one.

export interface RunTime {
  // The moment, written out for reading.
  absolute: string;
  // How long there is until it, empty when the moment cannot be read.
  relative: string;
}

const MINUTE = 60 * 1000;
const HOUR = 60 * MINUTE;
const DAY = 24 * HOUR;

// describeRun writes one run out. The locale and time zone are the
// browser's own unless a caller names them, which tests do so the wording
// does not follow the machine the tests run on.
export function describeRun(
  run: string,
  now: Date = new Date(),
  locale?: string,
  timeZone?: string,
): RunTime {
  const moment = new Date(run);
  if (Number.isNaN(moment.getTime())) return { absolute: run, relative: "" };
  const absolute = moment.toLocaleString(locale, {
    weekday: "short",
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
    timeZone,
  });
  return {
    absolute,
    relative: countdown(moment.getTime() - now.getTime(), locale),
  };
}

// countdown reads in the largest unit that still says something useful: a
// run under an hour away in minutes, one later the same day in hours, and
// anything further off in days. A run less than a minute away is one
// minute, never none: a schedule is due at most once a minute.
function countdown(until: number, locale?: string): string {
  const format = new Intl.RelativeTimeFormat(locale, { numeric: "auto" });
  if (until < HOUR)
    return format.format(Math.max(1, Math.round(until / MINUTE)), "minute");
  if (until < DAY) return format.format(Math.round(until / HOUR), "hour");
  return format.format(Math.round(until / DAY), "day");
}
