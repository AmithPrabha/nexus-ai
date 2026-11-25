package main

import (
	"strings"
	"testing"
)

func cleanFields() fields {
	f := fields{}
	values := []string{"Demo", "100123456789012", "ABC", "2026-01-01", "AED", "100.10", "0.05", "5.01", "105.11", "standard"}
	for i, k := range fieldNames {
		v := values[i]
		f[k] = &v
	}
	return f
}
func TestFinancialRules(t *testing.T) {
	for _, tc := range []struct{ k, v, code string }{
		{"total", "105.10", ""}, {"total", "105.09", "TOTAL_MISMATCH"}, {"total", "10000.01", "APPROVAL_THRESHOLD"},
		{"supplier_trn", " 100123456789012", "TRN_FORMAT"}, {"supplier_trn", "１００１２３４５６７８９０１２", "TRN_FORMAT"}, {"supplier_trn", "", "MISSING_SUPPLIER_TRN"},
		{"currency", "USD", "UNSUPPORTED_TAX"}, {"tax_treatment", "mixed", "UNSUPPORTED_TAX"}, {"vat_rate", "0", "UNSUPPORTED_RATE"},
		{"subtotal", "-1", "NEGATIVE_SUBTOTAL"}, {"vat_amount", "1e100000", "INVALID_VAT_AMOUNT"}, {"vat_amount", "4.99", "VAT_MISMATCH"},
	} {
		t.Run(tc.k+tc.v, func(t *testing.T) {
			f := cleanFields()
			f[tc.k] = &tc.v
			issues := validate(extraction{Fields: f}, false)
			var codes []string
			for _, r := range issues {
				codes = append(codes, r.Code)
			}
			if tc.code == "" && len(codes) > 0 || tc.code != "" && !strings.Contains(strings.Join(codes, ","), tc.code) {
				t.Fatalf("issues %v", codes)
			}
		})
	}
}
