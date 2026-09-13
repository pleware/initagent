package billing

import (
	"fmt"
	"strings"
	"unicode"
)

// Buyer is the fiscal recipient. KSeF needs a Polish company NIP.
type Buyer struct {
	Name     string `json:"name"`
	TaxNo    string `json:"taxNo"`
	Street   string `json:"street"`
	City     string `json:"city"`
	PostCode string `json:"postCode"`
	Country  string `json:"country"`
	Email    string `json:"email"`
}

// Validate requires a named buyer with an address. A PL buyer must carry
// a valid NIP so Fakturownia can send the invoice to KSeF.
func (b Buyer) Validate() error {
	if strings.TrimSpace(b.Name) == "" {
		return fmt.Errorf("%w: name", ErrBuyer)
	}
	if strings.TrimSpace(b.Street) == "" || strings.TrimSpace(b.City) == "" || strings.TrimSpace(b.PostCode) == "" {
		return fmt.Errorf("%w: address", ErrBuyer)
	}
	if strings.TrimSpace(b.Email) == "" || !strings.Contains(b.Email, "@") {
		return fmt.Errorf("%w: email", ErrBuyer)
	}
	country := b.NormCountry()
	if len(country) != 2 {
		return fmt.Errorf("%w: country", ErrBuyer)
	}
	if country == "PL" && !ValidNIP(b.TaxNo) {
		return fmt.Errorf("%w: NIP", ErrBuyer)
	}
	return nil
}

// NormCountry is the ISO country, default PL.
func (b Buyer) NormCountry() string {
	c := strings.ToUpper(strings.TrimSpace(b.Country))
	if c == "" {
		return "PL"
	}
	return c
}

// Company reports whether this buyer should go to KSeF as a firm.
func (b Buyer) Company() bool {
	return ValidNIP(b.TaxNo)
}

// ValidNIP reports a well-formed Polish NIP (10 digits + checksum).
func ValidNIP(raw string) bool {
	digits := make([]int, 0, 10)
	for _, r := range raw {
		if unicode.IsSpace(r) || r == '-' {
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
		digits = append(digits, int(r-'0'))
	}
	if len(digits) != 10 {
		return false
	}
	weights := [9]int{6, 5, 7, 2, 3, 4, 5, 6, 7}
	sum := 0
	for i, w := range weights {
		sum += digits[i] * w
	}
	check := sum % 11
	if check == 10 {
		return false
	}
	return check == digits[9]
}

// CleanNIP strips spaces and dashes. Empty when the value is not a NIP.
func CleanNIP(raw string) string {
	var b strings.Builder
	for _, r := range raw {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
