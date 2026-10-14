package worldcore

// Deposit is one rasterised `world:deposit`: a w×h int16 patch (HeightQ steps, added to h.land)
// whose cell (0, 0) is grid cell (X0, Z0). Bus payload: {id, x0, z0, w, h, cells: EncodeI16(Cells)}.
type Deposit struct {
	ID    string  `json:"id"`
	X0    int     `json:"x0"`
	Z0    int     `json:"z0"`
	W     int     `json:"w"`
	H     int     `json:"h"`
	Cells []int16 `json:"-"`
}

// RasterDeposit rasterises the deposit of event id at world (x, z), seeded by FNV-1a of id,
// size = f(heat), on top of solid = h.base + h.land. SIGNATURE IS SEAM; body: W1a lane A.
func RasterDeposit(id string, x, z, heat float64, solid []int16) Deposit {
	return Deposit{ID: id} // TODO(lane A)
}
