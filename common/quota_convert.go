package common

import "math"

// DisplayQuotaToRaw converts admin-friendly quota units into raw quota units.
func DisplayQuotaToRaw(display float64) int {
	return int(math.Round(display * QuotaPerUnit))
}

// RawQuotaToDisplay converts raw quota units into admin-friendly quota units.
func RawQuotaToDisplay(raw int) float64 {
	if QuotaPerUnit == 0 {
		return 0
	}
	return float64(raw) / QuotaPerUnit
}
