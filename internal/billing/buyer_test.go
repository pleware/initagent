package billing

import "testing"

func TestValidNIP(t *testing.T) {
	t.Parallel()
	// 5252445767 is the example NIP from Fakturownia's KSeF docs.
	if !ValidNIP("525-244-57-67") {
		t.Fatal("documented example must pass")
	}
	if !ValidNIP("5252445767") {
		t.Fatal("digits only must pass")
	}
	if ValidNIP("5252445768") {
		t.Fatal("bad checksum")
	}
	if ValidNIP("123") || ValidNIP("") || ValidNIP("abcdefghij") {
		t.Fatal("short or letters must fail")
	}
}

func TestBuyerValidate(t *testing.T) {
	t.Parallel()
	ok := Buyer{
		Kind:     KindCompany,
		Name:     "ACME Sp. z o.o.",
		TaxNo:    "5252445767",
		Street:   "Prosta 1",
		City:     "Warszawa",
		PostCode: "00-001",
		Country:  "pl",
		Email:    "fin@acme.test",
	}
	if err := ok.Validate(); err != nil {
		t.Fatal(err)
	}
	if !ok.Company() || ok.NormKind() != KindCompany || ok.NormCountry() != "PL" {
		t.Fatalf("company/country = %v %q", ok.Company(), ok.NormCountry())
	}
	bad := ok
	bad.TaxNo = "5252445768"
	if err := bad.Validate(); err == nil {
		t.Fatal("PL buyer without a real NIP must fail")
	}
	foreign := ok
	foreign.Country = "DE"
	foreign.TaxNo = ""
	if err := foreign.Validate(); err != nil {
		t.Fatalf("non-PL buyer may omit NIP: %v", err)
	}
	empty := Buyer{}
	if err := empty.Validate(); err == nil {
		t.Fatal("empty buyer")
	}
	noEmail := ok
	noEmail.Email = "not-an-email"
	if err := noEmail.Validate(); err == nil {
		t.Fatal("bad email must fail")
	}
	badCountry := ok
	badCountry.Country = "POL"
	if err := badCountry.Validate(); err == nil {
		t.Fatal("non-ISO country must fail")
	}
	noStreet := ok
	noStreet.Street = ""
	if err := noStreet.Validate(); err == nil {
		t.Fatal("missing street must fail")
	}
}

func TestBuyerIndividual(t *testing.T) {
	t.Parallel()
	person := Buyer{
		Kind:     KindIndividual,
		Name:     "Jan Kowalski",
		Street:   "Prosta 1",
		City:     "Warszawa",
		PostCode: "00-001",
		Country:  "pl",
		Email:    "jan@example.test",
	}
	if err := person.Validate(); err != nil {
		t.Fatalf("private person without NIP must pass: %v", err)
	}
	if person.Company() || person.NormKind() != KindIndividual {
		t.Fatalf("private person = company %v kind %q", person.Company(), person.NormKind())
	}
	// A PL private person never needs a NIP, even if the field is absent.
	if err := person.Validate(); err != nil {
		t.Fatal(err)
	}
	badKind := person
	badKind.Kind = "club"
	if err := badKind.Validate(); err == nil {
		t.Fatal("unknown kind must fail")
	}
}

func TestBuyerLegacyKindDefaultsCompany(t *testing.T) {
	t.Parallel()
	legacy := Buyer{
		Name:     "ACME",
		TaxNo:    "5252445767",
		Street:   "Prosta 1",
		City:     "Warszawa",
		PostCode: "00-001",
		Country:  "PL",
		Email:    "a@b.c",
	}
	if legacy.NormKind() != KindCompany || !legacy.Company() {
		t.Fatalf("legacy empty kind = %q company %v", legacy.NormKind(), legacy.Company())
	}
	if err := legacy.Validate(); err != nil {
		t.Fatalf("legacy company must pass: %v", err)
	}
}

func TestCleanNIP(t *testing.T) {
	t.Parallel()
	if got := CleanNIP("525-244 57 67"); got != "5252445767" {
		t.Fatalf("CleanNIP = %q", got)
	}
}
