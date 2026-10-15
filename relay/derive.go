package main

// World frames derived from events (06 worldcore list: vein capsules, glacier state, ash lines, snowline).
// Orchestrator-owned (W3b seam). The geometry and the rules live in worldcore; this file only maps an ev to
// the world frames it causes. Called from the hub tap (App.record) with h.mu and a.mu held, so the derived
// frames follow their ev atomically; they reach the state like any frame, through the tap (State.apply).

import "github.com/MohammadBnei/mountain/relay/worldcore"

// derive returns the world frames ev f causes (nil = none). Caller holds h.mu and a.mu.
func (a *App) derive(f frameF) [][]byte {
	o, g := &Out{app: a, name: "world"}, a.State.Glacier
	glacier := func(g worldcore.Glacier) []byte {
		return o.worldFrame("glacier", map[string]any{"mass": g.Mass, "tongues": g.Tongues, "ash": g.Ash})
	}
	switch f.Kind {
	case "alert.resolve": // 02: a vein needs severity ≥ warning and a duration ≥ 10 min
		if f.Sev == "info" || f.DurS < worldcore.VeinMin {
			return nil
		}
		v := worldcore.PlaceVein(f.FP, f.Layer, f.DurS, worldcore.Solid(a.World.Base, a.Land))
		l := worldcore.AddVein(a.State.Veins, v)
		v = l[len(l)-1] // a re-fired fp keeps its seam and the thicker radius
		return [][]byte{o.worldFrame("vein", map[string]any{"fp": v.FP, "a": v.A[:], "b": v.B[:], "thick": v.Thick, "layer": v.Layer, "ref": v.Ref})}
	case "argo.sync":
		if f.OK {
			return [][]byte{glacier(g.Advance(f.App))}
		}
	case "app.gone":
		if g, ok := g.Calve(f.App); ok {
			return [][]byte{glacier(g)}
		}
	case "gitops.commit":
		return [][]byte{glacier(g.Snow(f.N))}
	case "adr.merge":
		var out [][]byte
		if _, ok := worldcore.AddAshLine(a.State.Ash, worldcore.Ash{Day: f.Day, Title: f.Title}); ok {
			ash := map[string]any{"day": f.Day}
			if f.Title != "" {
				ash["title"] = f.Title
			}
			out = append(out, o.worldFrame("ash", ash))
		}
		if g, ok := g.AddAsh(f.Day); ok {
			out = append(out, glacier(g))
		}
		return out
	}
	return nil
}

// melt is the daily glacier ablation (persist, at UTC rollover): one world:glacier through the hub.
func (a *App) melt() {
	a.Hub.mu.Lock()
	defer a.Hub.mu.Unlock()
	a.mu.Lock()
	g := a.State.Glacier.Melt()
	a.mu.Unlock()
	a.Hub.broadcastLocked((&Out{app: a, name: "world"}).worldFrame("glacier", map[string]any{"mass": g.Mass, "tongues": g.Tongues, "ash": g.Ash}))
}
