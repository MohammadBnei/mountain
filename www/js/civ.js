// R9 Civ: the rules the page shares (relay civ.go keeps the data). Every word comes in en and fr; status is said in
// words (Ukubi), the Civ names only dress the chrome.
//   goldenAge(world) → {on, days}: calm (state.calm) for GOLDEN_DAYS or more, judged on the replayed day during a scrub
//   turnOf(world, started) → n | null: the town's day count (started = hello.snapshot.started), day one = 1
//   today(world) → 'YYYY-MM-DD' (UTC): the replayed day during a scrub, else now
export const GOLDEN_DAYS = 7;
export const LEVEL_AT = [4, 12]; // relay civ.go levelAt: successful syncs in 30 days for level 2 and 3 (the guide says so)
const DAY = 86400000;

export const today = (world) => world.replay?.day || new Date().toISOString().slice(0, 10);
const days = (from, to) => Math.round((Date.parse(to) - Date.parse(from)) / DAY);

export function goldenAge(world) {
  const calm = world.state?.calm;
  if (!calm) return { on: false, days: 0 };
  const n = days(calm, today(world));
  return { on: n >= GOLDEN_DAYS, days: n };
}

export function turnOf(world, started) {
  return started ? days(started, today(world)) + 1 : null;
}

/** The herald's words for a decree (kind: sync | laws | tech | level), in en and fr. */
export function decree(kind, { name, n, level } = {}, fr = false) {
  const num = n != null ? `ADR-${String(n).padStart(4, '0')}` : '';
  switch (kind) {
    case 'sync': return fr ? `Décret : ${name} mis à jour` : `Decree: ${name} updated`;
    case 'laws': return fr ? 'Nouvelles lois votées' : 'New laws passed';
    case 'tech': return fr ? `Technologie découverte : ${num}` : `Technology discovered: ${num}`;
    case 'level': return fr ? `${name} grandit (niveau ${level})` : `${name} grows (level ${level})`;
  }
  return '';
}
