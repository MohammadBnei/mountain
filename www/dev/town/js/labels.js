// Building names (R7): one small HTML label per plot, projected every frame. HTML, not sprites: it stays sharp
// and readable at phone size, and costs no draw call. Labels never overlap: nearest first, apps before services,
// a label that would cover one already placed waits until the camera comes closer. A ruin says so.
const SINK = 0.5; // islands.js: a down island sinks this far

export function createLabels({ THREE, layer, apps, lang }) {
  const els = new Map(), v = new THREE.Vector3(), placed = [];
  return {
    update(plots, camera, w, h) {
      const seen = new Set(), cand = [];
      for (const p of plots) {
        seen.add(p.app);
        let el = els.get(p.app);
        if (!el) { el = document.createElement('span'); el.className = 'label'; layer.append(el); els.set(p.app, el); }
        const a = apps[p.app] || {}, name = a.name || p.app;
        const text = p.ruinAt ? `${name} · ${lang() === 'fr' ? 'ruines' : 'gone'}` : name;
        if (el.textContent !== text) { el.textContent = text; el._w = 0; }
        el.classList.toggle('service', a.group === 'service');
        el.style.setProperty('--wire', `var(--wire-${a.wire || 'app'})`);
        el.classList.toggle('down', !!p.down);
        el.classList.toggle('gone', !!p.ruinAt);
        v.set(p.x, (p.ruinAt ? 0.5 : p.k.height) + 0.35 + (p.down ? -SINK : 0), p.z);
        const d = v.distanceTo(camera.position);
        v.project(camera);
        if (v.z >= 1 || Math.abs(v.x) > 1.1 || Math.abs(v.y) > 1.1) { el.hidden = true; continue; }
        cand.push({ el, x: ((v.x + 1) / 2) * w, y: ((1 - v.y) / 2) * h, d, svc: a.group === 'service' });
      }
      cand.sort((a, b) => a.svc - b.svc || a.d - b.d);
      placed.length = 0;
      for (const c of cand) {
        const cw = (c.el._w ||= c.el.offsetWidth || 80) + 4, ch = 22;
        const r = [c.x - cw / 2, c.y - ch, c.x + cw / 2, c.y];
        const hit = placed.some((o) => r[0] < o[2] && r[2] > o[0] && r[1] < o[3] && r[3] > o[1]);
        c.el.hidden = hit;
        if (hit) continue;
        placed.push(r);
        c.el.style.transform = `translate(${c.x}px, ${c.y}px) translate(-50%, -100%)`;
      }
      for (const [app, el] of els) if (!seen.has(app)) { el.remove(); els.delete(app); }
    },
  };
}
