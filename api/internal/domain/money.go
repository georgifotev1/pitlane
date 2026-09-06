package domain

const StandardVATRateBPS = 2000

// TaxCents extracts rounded VAT from a VAT-inclusive amount.
func TaxCents(totalCents int64, taxRateBps int) int64 {
	denominator := int64(10000) + int64(taxRateBps)
	return (totalCents*int64(taxRateBps) + denominator/2) / denominator
}

// NetCents strips VAT from a VAT-inclusive amount, the complement of TaxCents.
func NetCents(totalCents int64, taxRateBps int) int64 {
	return totalCents - TaxCents(totalCents, taxRateBps)
}

// MarginBps expresses profit as a share of net revenue, in basis points. Zero
// revenue has no margin to report.
func MarginBps(profitCents, netRevenueCents int64) int {
	if netRevenueCents <= 0 {
		return 0
	}
	return int(profitCents * 10000 / netRevenueCents)
}
