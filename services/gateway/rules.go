package main

import (
	"regexp"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

type fields map[string]*string
type evidence struct {
	Page    int    `json:"page"`
	Excerpt string `json:"excerpt"`
}
type extraction struct {
	Fields   fields              `json:"fields"`
	Evidence map[string]evidence `json:"evidence"`
	Warnings []string            `json:"warnings"`
}
type rule struct {
	Code     string `json:"rule_code"`
	Severity string `json:"severity"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Message  string `json:"message"`
}

var fieldNames = []string{"supplier_name", "supplier_trn", "invoice_number", "invoice_date", "currency", "subtotal", "vat_rate", "vat_amount", "total", "tax_treatment"}
var digits = regexp.MustCompile("^[0-9]{15}$")
var numeric = regexp.MustCompile("^-?[0-9]{1,16}([.][0-9]{1,6})?$")
var tolerance = decimal.RequireFromString("0.01")
var standardRate = decimal.RequireFromString("0.05")
var threshold = decimal.NewFromInt(10000)

func (f fields) value(key string) string {
	if f[key] == nil {
		return ""
	}
	return strings.TrimSpace(*f[key])
}
func validFields(f fields) bool {
	for k, v := range f {
		known := false
		for _, name := range fieldNames {
			if name == k {
				known = true
				break
			}
		}
		if !known || (v != nil && len(*v) > 256) {
			return false
		}
	}
	return true
}
func validate(x extraction, duplicate bool) []rule {
	results := []rule{}
	issue := func(code, expected, actual, message string) {
		results = append(results, rule{code, "review", expected, actual, message})
	}
	for _, key := range fieldNames {
		if x.Fields.value(key) == "" {
			issue("MISSING_"+strings.ToUpper(key), "present", "missing", "Missing "+key+".")
		}
	}
	if raw := x.Fields["supplier_trn"]; raw != nil && strings.TrimSpace(*raw) != "" && !digits.MatchString(*raw) {
		trn := *raw
		issue("TRN_FORMAT", "15 ASCII digits", trn, "Check the supplier TRN format.")
	}
	if date := x.Fields.value("invoice_date"); date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			issue("INVOICE_DATE", "YYYY-MM-DD", date, "Check the invoice date.")
		}
	}
	currency, treatment := x.Fields.value("currency"), x.Fields.value("tax_treatment")
	if currency != "AED" || treatment != "standard" {
		issue("UNSUPPORTED_TAX", "AED / standard", currency+" / "+treatment, "This tax treatment or currency needs manual review.")
	}
	amounts := map[string]decimal.Decimal{}
	for _, key := range []string{"subtotal", "vat_rate", "vat_amount", "total"} {
		value := x.Fields.value(key)
		if value == "" {
			continue
		}
		if !numeric.MatchString(value) {
			issue("INVALID_"+strings.ToUpper(key), "decimal", value, "Invalid decimal amount.")
			continue
		}
		n, _ := decimal.NewFromString(value)
		amounts[key] = n
		if n.IsNegative() {
			issue("NEGATIVE_"+strings.ToUpper(key), "nonnegative", value, "Negative values need review.")
		}
	}
	if rate, ok := amounts["vat_rate"]; ok && !rate.Equal(standardRate) {
		issue("UNSUPPORTED_RATE", "0.05", rate.String(), "Only the standard demo rate supports automatic arithmetic.")
	}
	if currency == "AED" && treatment == "standard" && amounts["vat_rate"].Equal(standardRate) {
		sub, okSub := amounts["subtotal"]
		vat, okVAT := amounts["vat_amount"]
		total, okTotal := amounts["total"]
		compare := func(code string, want, got decimal.Decimal) {
			if want.Sub(got).Abs().GreaterThan(tolerance) {
				issue(code, want.StringFixed(2), got.String(), "The reported amount differs by more than AED 0.01.")
			}
		}
		if okSub && okVAT {
			compare("VAT_MISMATCH", sub.Mul(standardRate).Round(2), vat)
		}
		if okSub && okVAT && okTotal {
			compare("TOTAL_MISMATCH", sub.Add(vat).Round(2), total)
		}
	}
	if currency == "AED" && amounts["total"].GreaterThan(threshold) {
		issue("APPROVAL_THRESHOLD", "10000.00", amounts["total"].String(), "Above the fictional company review threshold.")
	}
	if duplicate {
		issue("DUPLICATE_INVOICE", "unique supplier and invoice", "duplicate", "A matching invoice exists in this workspace.")
	}
	for _, warning := range x.Warnings {
		issue("EXTRACTION_WARNING", "verified evidence", warning, "Review the extraction warning.")
	}
	return results
}
