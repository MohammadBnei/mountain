// Building names (R7): one small HTML label per plot, projected every frame. HTML, not sprites: it stays sharp
// and readable at phone size, and costs no draw call. Labels never overlap: nearest first, apps before services,
// a label that would cover one already placed waits until the camera comes closer. A ruin says so.
// R8: the dot's shape says the class (round: an app, diamond: a platform service, square: a machine), its colour the
// wire. Apps are placed first, then services, then machines, which are named only once the camera leaves the
// overview. The dock is an obstacle: no label hides behind the panel. chips (the phone's overview, where forty names
// would be noise): one label per island instead, "h1 · 12 buildings", and no building names.
// R8 C: a tent (an app's annex on another island) carries its app's name once the camera leaves the overview, like a
// machine, and is placed last: it never takes a building's place.
const SINK = 0.5; // islands.js: a down island sinks this far

export function createLabels({ THREE, layer, apps, lang }) {
  const els = new Map(), v = new THREE.Vector3(), placed = [];
  document.fonts?.ready.then(() => { for (const el of els.values()) el._w = 0; }); // widths measured before the web font swap in
  return {
    /** plots: buildings.plots(); obstacle: [x0, y0, x1, y1] in canvas px (the dock); chips: islands → island labels;
     *  tents: buildings.annexes(), named with the machines (machines = false hides both); notes: the herald's decrees
     *  [{key, host, x, y, z, text}], placed first (in the chips' overview, a decree takes its island's chip). */
    update(plots, camera, w, h, { obstacle, chips, machines = true, tents = [], notes = [] } = {}) {
      const seen = new Set(), cand = [];
      const said = new Map(notes.map((n) => [n.host, n.text]));
      for (const n of chips ? [] : notes) {
        seen.add(n.key);
        let el = els.get(n.key);
        if (!el) { el = document.createElement('span'); el.className = 'label herald'; layer.append(el); els.set(n.key, el); }
        if (el.textContent !== n.text) { el.textContent = n.text; el._w = 0; }
        v.set(n.x, n.y + (n.down ? -SINK : 0), n.z);
        const d = v.distanceTo(camera.position);
        v.project(camera);
        if (v.z >= 1 || Math.abs(v.x) > 1.1 || Math.abs(v.y) > 1.1) { el.hidden = true; continue; }
        cand.push({ el, x: ((v.x + 1) / 2) * w, y: ((1 - v.y) / 2) * h, d, svc: -1 });
      }
      if (chips) {
        for (const is of chips) {
          const key = `@${is.host}`, n = plots.filter((p) => p.host === is.host && !p.ruinAt).length;
          seen.add(key);
          let el = els.get(key);
          if (!el) { el = document.createElement('span'); el.className = 'label chip'; layer.append(el); els.set(key, el); }
          const text = said.get(is.host) || `${is.host} · ${n} ${lang() === 'fr' ? (n === 1 ? 'bâtiment' : 'bâtiments') : (n === 1 ? 'building' : 'buildings')}`; // R9: a decree takes the chip
          if (el.textContent !== text) { el.textContent = text; el._w = 0; }
          v.set(is.centre[0], 3, is.centre[1]);
          const d = v.distanceTo(camera.position);
          v.project(camera);
          if (v.z >= 1 || Math.abs(v.x) > 1.1 || Math.abs(v.y) > 1.1) { el.hidden = true; continue; }
          cand.push({ el, x: ((v.x + 1) / 2) * w, y: ((1 - v.y) / 2) * h, d, svc: false });
        }
      }
      for (const p of chips ? [] : plots) {
        seen.add(p.app);
        let el = els.get(p.app);
        if (!el) { el = document.createElement('span'); el.className = 'label'; layer.append(el); els.set(p.app, el); }
        const a = apps[p.app] || {}, name = a.name || p.app;
        const text = p.ruinAt ? `${name} · ${lang() === 'fr' ? 'ruines' : 'gone'}` : name;
        if (el.textContent !== text) { el.textContent = text; el._w = 0; }
        el.classList.toggle('service', a.group === 'service');
        el.classList.toggle('machine', a.group === 'machine');
        el.style.setProperty('--wire', `var(--wire-${a.wire || 'app'})`);
        el.classList.toggle('down', !!p.down || !!p.off);
        el.classList.toggle('gone', !!p.ruinAt);
        if (a.group === 'machine' && !machines) { el.hidden = true; continue; } // R8: machines are named up close only
        v.set(p.x, (p.ruinAt ? 0.5 : p.k.height) + 0.35 + (p.down ? -SINK : 0), p.z);
        const d = v.distanceTo(camera.position);
        v.project(camera);
        if (v.z >= 1 || Math.abs(v.x) > 1.1 || Math.abs(v.y) > 1.1) { el.hidden = true; continue; }
        cand.push({ el, x: ((v.x + 1) / 2) * w, y: ((1 - v.y) / 2) * h, d, svc: { service: 1, machine: 2 }[a.group] || 0 });
      }
      for (const x of chips || !machines ? [] : tents) {
        seen.add(x.key);
        let el = els.get(x.key);
        if (!el) { el = document.createElement('span'); el.className = 'label tent'; layer.append(el); els.set(x.key, el); }
        const a = apps[x.app] || {}, text = a.name || x.app;
        if (el.textContent !== text) { el.textContent = text; el._w = 0; }
        el.style.setProperty('--wire', `var(--wire-${a.wire || 'app'})`);
        el.classList.toggle('down', !!x.down);
        v.set(x.x, x.k.height + 0.3 + (x.down ? -SINK : 0), x.z);
        const d = v.distanceTo(camera.position);
        v.project(camera);
        if (v.z >= 1 || Math.abs(v.x) > 1.1 || Math.abs(v.y) > 1.1) { el.hidden = true; continue; }
        cand.push({ el, x: ((v.x + 1) / 2) * w, y: ((1 - v.y) / 2) * h, d, svc: 3 });
      }
      cand.sort((a, b) => a.svc - b.svc || a.d - b.d); // the herald first, then apps, services, machines, tents
      placed.length = 0;
      if (obstacle) placed.push(obstacle);
      for (const c of cand) {
        if (!c.el._w) { c.el.hidden = false; c.el._w = c.el.offsetWidth; } // a hidden label measures 0: show it to measure
        const cw = (c.el._w || 80) + 4, ch = 22;
        c.x = Math.min(w - cw / 2 - 2, Math.max(cw / 2 + 2, c.x)); // inside the canvas: a long decree near an edge stays readable
        const r = [c.x - cw / 2, c.y - ch, c.x + cw / 2, c.y];
        const hit = placed.some((o) => r[0] < o[2] && r[2] > o[0] && r[1] < o[3] && r[3] > o[1]);
        c.el.hidden = hit;
        if (hit) continue;
        placed.push(r);
        c.el.style.transform = `translate(${c.x}px, ${c.y}px) translate(-50%, -100%)`;
      }
      for (const [key, el] of els) if (!seen.has(key)) { el.remove(); els.delete(key); }
    },
  };
}
