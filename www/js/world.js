// The town's world (R7): the relay's state mirrored, and a bus. Folds frames with the relay's rules
// (relay/sources.go State.apply), so a reload, a resync and a replay show the same town.
//
//   createWorld() → world
//   world.town       hello.town: {islands:[{host, q, r, radius, tiles}], bridges:[{a, b, aq, ar, bq, br}]} (R8: a radius per island)
//   world.build      hello.build: the relay's www build id (R8; main.js reloads a page from another build)
//   world.state      {alerts, health, replicas, builds, marks, plots, machines, annexes}: hello.state, then every frame, BEFORE its emit
//   world.metrics    the last metrics frame;  world.stale  null | unix ms (oldest stale source, or the socket drop)
//   world.ready      host → bool (ev node.ready, metrics.hosts; absent = not measured yet: draw it up)
//   world.quorum     [hosts] | null (hello.state.quorum, then ev quorum; null = not known yet)
//   world.on(type, fn) → off   types: 'hello', 'metrics', 'ev:<kind>', 'world:<kind>', 'state' (after any change)
//   world.applyHello(h) / world.applyFrame(f)   (socket.js)
//   world.place(mark, pose) → Promise<id>  /  world.unmark(id) → Promise<id>   (visitor marks; rejects with the reason)
//   world.scrub(day 'YYYY-MM-DD') → Promise<replay>: the relay's replay of that day (06): the town at the day's start,
//                then its frames played over ≤ 20 s by step(dt); live frames are dropped while it is held. Rejects on
//                {error} (bad-day | rate | unavailable: before the town began, or no database), offline, or 10 s.
//   world.release() → back to live (a resync: the next hello);  world.replay  null | {day, left}
//   world.step(dt)  plays a held replay (main.js calls it every frame)
//   world.send / world.resync   wired by socket.connect
export function createWorld() {
  const subs = new Map(), acks = new Map();
  let refN = 0, resolveReady, play = null; // play: {day, queue, gap, t, timer, done: [resolve, reject]}
  const emptyState = () => ({ alerts: [], health: {}, replicas: {}, builds: {}, marks: [], plots: [], machines: {}, annexes: [] });
  const w = {
    town: null, build: null, state: emptyState(), metrics: null, stale: null, ready: {}, quorum: null, quota: null, named: false, replay: null,
    loaded: new Promise((r) => (resolveReady = r)),
    on(type, fn) { if (!subs.has(type)) subs.set(type, new Set()); subs.get(type).add(fn); return () => subs.get(type).delete(fn); },
    emit(type, payload) { for (const fn of subs.get(type) || []) fn(payload, type); },

    applyHello(h) {
      if (h.town) w.town = h.town;
      w.build = h.build ?? null;
      w.state = { ...emptyState(), ...h.state };
      for (const k of Object.keys(emptyState())) if (w.state[k] == null) w.state[k] = emptyState()[k];
      w.quorum = h.state?.quorum ?? null; // R7.1: rides the hello (null = not known yet)
      if (h.metrics) metrics(h.metrics);
      w.emit('hello', h);
      w.emit('state', w.state);
      resolveReady(w);
    },
    applyFrame(f) {
      if (f.t === 'replay') return replay(f);
      if (f.t === 'hello') { if (play) { play.done[1](new Error('resync')); clearTimeout(play.timer); play = null; w.replay = null; } return w.applyHello(f); }
      if (play && (f.t === 'ev' || f.t === 'world' || f.t === 'metrics')) return; // held on a past day
      if (f.t === 'mark') return answer(f);
      if (f.t === 'metrics') { metrics(f); return w.emit('metrics', f); }
      if (f.t === 'ev') { fold(f); w.emit(`ev:${f.kind}`, f); return w.emit('state', w.state); }
      if (f.t === 'world') {
        const { t, kind, ...p } = f;
        fold(f, p);
        w.emit(`world:${kind}`, p);
        return w.emit('state', w.state);
      }
    },
    place(mark, pose) { return ask({ t: 'mark', mark, pose, named: w.named }); },
    scrub(day) {
      return new Promise((resolve, reject) => {
        if (!w.send) return reject(new Error('offline: no socket'));
        if (play) { clearTimeout(play.timer); play.done[1](new Error('superseded')); }
        play = { day, queue: [], gap: 0.25, t: 0, done: [resolve, reject], timer: setTimeout(() => { play = null; w.replay = null; reject(new Error('timeout')); w.resync?.(); }, 10000) };
        if (!w.send({ t: 'scrub', day })) { clearTimeout(play.timer); play = null; reject(new Error('offline')); }
      });
    },
    release() { if (play) clearTimeout(play.timer); play = null; w.replay = null; w.resync?.(); },
    step(dt) {
      if (!play?.queue.length) return;
      for (play.t += dt; play.t >= play.gap && play.queue.length; play.t -= play.gap) {
        const f = play.queue.shift();
        if (f.t === 'ev') { fold(f); w.emit(`ev:${f.kind}`, f); } else if (f.t === 'world') { const { t, kind, ...p } = f; fold(f, p); w.emit(`world:${kind}`, p); }
        w.replay.left = play.queue.length;
      }
      w.emit('state', w.state);
    },
    unmark(id) { return ask({ t: 'mark.delete', id }); },
  };

  function metrics(f) {
    w.metrics = f;
    for (const [h, x] of Object.entries(f.hosts || {})) if (x.ready != null) w.ready[h] = !!x.ready; // null = not measured yet: unknown, not down
    const st = Object.values(f.stale || {});
    w.stale = st.length ? Math.min(...st) : null;
  }

  // The relay's State.apply, mirrored.
  function fold(f, p) {
    const s = w.state;
    if (f.t === 'ev') {
      switch (f.kind) {
        case 'alert.fire':
          s.alerts = s.alerts.filter((a) => a.fp !== f.fp);
          s.alerts.push({ fp: f.fp, sev: f.sev, layer: f.layer, startsAt: f.startsAt, ...(f.host && { host: f.host }), ...(f.app && { app: f.app }) });
          break;
        case 'alert.resolve': s.alerts = s.alerts.filter((a) => a.fp !== f.fp); break;
        case 'argo.health': s.health[f.app] = f.status; break;
        case 'replicas': s.replicas[f.app] = f.bucket; break;
        case 'build': if (f.state === 'queued' || f.state === 'running') s.builds[f.app] = f.state; else delete s.builds[f.app]; break;
        case 'app.gone': for (const k of ['health', 'replicas', 'builds', 'machines']) delete s[k][f.app]; break;
        case 'machine': s.machines[f.app] = f.state; break; // R8: a probed machine's up|down, after 10 min held
        case 'node.ready': w.ready[f.host] = f.ready; break;
        case 'quorum': w.quorum = s.quorum = f.up; break;
      }
      return;
    }
    if (f.kind === 'mark') {
      s.marks = s.marks.filter((m) => m.id !== p.id);
      if (!p.gone) s.marks = s.marks.concat(p).slice(-2000);
    }
    if (f.kind === 'plot') {
      s.plots = s.plots.filter((x) => x.app !== p.app);
      if (!p.gone) { const { gone, ...plot } = p; s.plots = s.plots.concat(plot).slice(-64); }
    }
    if (f.kind === 'annex') { // R8 C: one tent per app@host (relay annexes.go)
      s.annexes = s.annexes.filter((x) => x.app !== p.app || x.host !== p.host);
      if (!p.gone) { const { gone, ...tent } = p; s.annexes = s.annexes.concat(tent).slice(-48); }
    }
  }

  /** The relay's replay of a day: the town at its start, then its frames (step plays them). */
  function replay(f) {
    if (!play || (f.day ? f.day !== play.day : !f.error)) return; // released, or an earlier request's answer
    const [ok, no] = play.done;
    clearTimeout(play.timer);
    if (f.error) { play = null; w.replay = null; w.resync?.(); return no(new Error(f.error)); }
    w.state = { ...emptyState(), ...f.state };
    for (const k of Object.keys(emptyState())) if (w.state[k] == null) w.state[k] = emptyState()[k]; // a day from before a field
    w.quorum = f.state?.quorum ?? null;
    play.queue = f.frames.slice(); play.gap = Math.min(0.25, 20 / Math.max(1, f.frames.length));
    w.replay = { day: f.day, left: f.frames.length };
    w.emit('state', w.state);
    ok(f);
  }
  function ask(f) {
    const ref = `r${(++refN).toString(36)}`;
    return new Promise((resolve, reject) => {
      if (!w.send?.({ ...f, ref })) return reject(new Error('offline'));
      acks.set(ref, [resolve, reject, setTimeout(() => { acks.delete(ref); reject(new Error('timeout')); }, 10000)]);
    });
  }
  function answer(f) {
    const a = acks.get(f.ref);
    if (!a) return;
    acks.delete(f.ref); clearTimeout(a[2]);
    if (f.next != null) w.quota = { next: f.next, left: f.left, at: Date.now() };
    if (f.error) a[1](new Error(f.error)); else a[0](f.id);
  }
  return w;
}
