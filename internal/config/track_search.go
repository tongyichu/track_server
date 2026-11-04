package config

// SearchDistanceBreakpoints are product presets in meters, keyed by stable activity code.
func SearchDistanceBreakpoints() map[string][]float64 {
	return map[string][]float64{
		"":         {5000, 10000, 20000, 50000},
		"hiking":   {5000, 10000, 20000},
		"climbing": {3000, 5000, 10000, 15000},
		"running":  {3000, 5000, 10000, 21000},
		"riding":   {10000, 30000, 60000, 100000},
		"driving":  {50000, 100000, 300000},
	}
}
