// R9 Civ: the rules the page shares (relay civ.go keeps the data). Every word comes in en and fr; status is said in
// words (Ukubi), the Civ names only dress the chrome.
//   goldenAge(world) → {on, days}: on after GOLDEN_DAYS whole days of calm (state.calm is the day it began, a part day);
//   days = the golden age's own day (1 on its first). Judged on the replayed day during a scrub
//   turnOf(world, started) → n | null: the town's day count (started = hello.snapshot.started), day one = 1
//   today(world) → 'YYYY-MM-DD' (UTC): the replayed day during a scrub, else now
export const GOLDEN_DAYS = 7;
const DAY = 86400000;

export const today = (world) => world.replay?.day || new Date().toISOString().slice(0, 10);
const days = (from, to) => Math.round((Date.parse(to) - Date.parse(from)) / DAY);

export function goldenAge(world) {
  const calm = world.state?.calm;
  if (!calm) return { on: false, days: 0 };
  const n = days(calm, today(world)) - GOLDEN_DAYS; // whole calm days past GOLDEN_DAYS: calm's own day never counts
  return { on: n > 0, days: Math.max(0, n) };
}

export function turnOf(world, started) {
  return started ? days(started, today(world)) + 1 : null;
}

/** The herald's words for a decree (kind: sync | laws | tech), in en and fr. */
export function decree(kind, { name, n } = {}, fr = false) {
  const num = n != null ? `ADR-${String(n).padStart(4, '0')}` : '';
  switch (kind) {
    case 'sync': return fr ? `Décret : ${name} mis à jour` : `Decree: ${name} updated`;
    case 'laws': return fr ? 'Nouvelles lois votées' : 'New laws passed';
    case 'tech': return fr ? `Technologie découverte : ${num}` : `Technology discovered: ${num}`;
  }
  return '';
}
