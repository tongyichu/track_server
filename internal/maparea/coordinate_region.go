package maparea

// UsesGCJ02 describes coordinate conversion coverage, independently of provider routing.
func UsesGCJ02(latitude, longitude float64) bool {
	// District bounds from districts.json, expanded by 0.01 degrees so the
	// WGS84/GCJ02 offset and coastal tracks cannot straddle a conversion seam.
	// These small envelopes contain no neighboring foreign land; the mainland
	// still uses its polygon mask rather than a China-wide rectangle.
	hongKong := Bounds{MinLatitude: 22.141076, MinLongitude: 113.829859, MaxLatitude: 22.569092, MaxLongitude: 114.457264}
	macau := Bounds{MinLatitude: 22.096214, MinLongitude: 113.523314, MaxLatitude: 22.224074, MaxLongitude: 113.613873}
	return hongKong.contains(latitude, longitude) || macau.contains(latitude, longitude) || IsMainland(latitude, longitude)
}
