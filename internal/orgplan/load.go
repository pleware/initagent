package orgplan

import (
	"bytes"
	"fmt"

	"gopkg.in/yaml.v3"
)

type catalogFile struct {
	Config catalogConfig `yaml:"config"`
}

type catalogConfig struct {
	PlansOrder []string            `yaml:"plansOrder"`
	Plans      map[string]planYAML `yaml:"plans"`
}

type planYAML struct {
	SelfServe   bool        `json:"selfServe" yaml:"selfServe"`
	Charge      Charge      `json:"charge" yaml:"charge"`
	ThemeFamily ThemeFamily `json:"themeFamily" yaml:"themeFamily"`
	Limits      Limits      `json:"limits" yaml:"limits"`
}

// Load parses a catalogue YAML document. It does not consult the embedded
// file; tests pass fixtures. Unknown fields, unknown slugs, and a set of
// slugs that does not match the typed ids all fail.
func Load(data []byte) ([]Plan, int, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, 0, fmt.Errorf("orgplan: empty catalogue")
	}
	var file catalogFile
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&file); err != nil {
		return nil, 0, fmt.Errorf("orgplan: parse catalogue: %w", err)
	}
	order := file.Config.PlansOrder
	raw := file.Config.Plans
	if len(order) == 0 {
		return nil, 0, fmt.Errorf("orgplan: config.plansOrder is empty")
	}
	if raw == nil {
		return nil, 0, fmt.Errorf("orgplan: config.plans is missing")
	}
	seen := make(map[string]bool, len(order))
	for _, slug := range order {
		if slug == "" {
			return nil, 0, fmt.Errorf("orgplan: empty slug in plansOrder")
		}
		if seen[slug] {
			return nil, 0, fmt.Errorf("orgplan: duplicate slug %q in plansOrder", slug)
		}
		seen[slug] = true
		if _, ok := raw[slug]; !ok {
			return nil, 0, fmt.Errorf("orgplan: plansOrder slug %q missing from config.plans", slug)
		}
	}
	for slug := range raw {
		if !seen[slug] {
			return nil, 0, fmt.Errorf("orgplan: config.plans slug %q missing from plansOrder", slug)
		}
	}

	want := ids()
	if len(order) != len(want) {
		return nil, 0, fmt.Errorf("orgplan: %d slugs, want %d typed ids", len(order), len(want))
	}
	byID := make(map[ID]bool, len(want))
	for _, id := range want {
		byID[id] = true
	}

	plans := make([]Plan, 0, len(order))
	var personUSD int
	var sawUSD bool
	for _, slug := range order {
		id := ID(slug)
		if !byID[id] {
			return nil, 0, fmt.Errorf("orgplan: unknown slug %q", slug)
		}
		row := raw[slug]
		if err := validateRow(id, row); err != nil {
			return nil, 0, err
		}
		if row.Charge.Kind == ChargeUSD {
			if !sawUSD {
				personUSD = row.Charge.USD
				sawUSD = true
			} else if row.Charge.USD != personUSD {
				return nil, 0, fmt.Errorf("orgplan: %s usd %d, want %d (one advertised person price)", slug, row.Charge.USD, personUSD)
			}
		}
		plans = append(plans, Plan{
			ID:          id,
			SelfServe:   row.SelfServe,
			Charge:      row.Charge,
			ThemeFamily: row.ThemeFamily,
			Limits:      row.Limits,
		})
	}
	for _, id := range want {
		if _, ok := raw[string(id)]; !ok {
			return nil, 0, fmt.Errorf("orgplan: typed id %q missing from catalogue", id)
		}
	}
	if !sawUSD {
		return nil, 0, fmt.Errorf("orgplan: no usd plan to advertise PersonUSD")
	}
	return plans, personUSD, nil
}

func validateRow(id ID, row planYAML) error {
	switch row.Charge.Kind {
	case ChargeFree:
		if row.Charge.USD != 0 || row.Charge.PerPerson {
			return fmt.Errorf("orgplan: %s: free charge must be usd 0 and not perPerson", id)
		}
		if !row.SelfServe {
			return fmt.Errorf("orgplan: %s: free must be self-serve", id)
		}
	case ChargeUSD:
		if row.Charge.USD <= 0 || !row.Charge.PerPerson {
			return fmt.Errorf("orgplan: %s: usd charge must be usd > 0 and perPerson", id)
		}
		if !row.SelfServe {
			return fmt.Errorf("orgplan: %s: usd plan must be self-serve", id)
		}
	case ChargeContact:
		if row.Charge.USD != 0 || row.Charge.PerPerson {
			return fmt.Errorf("orgplan: %s: contact charge must be usd 0 and not perPerson", id)
		}
		if row.SelfServe {
			return fmt.Errorf("orgplan: %s: contact-sales must not be self-serve", id)
		}
	default:
		return fmt.Errorf("orgplan: %s: unknown charge kind %q", id, row.Charge.Kind)
	}
	switch row.ThemeFamily {
	case ThemeDefault, ThemeEnterprise:
	default:
		return fmt.Errorf("orgplan: %s: unknown themeFamily %q", id, row.ThemeFamily)
	}
	if row.Limits.Projects < 0 || row.Limits.WorkersPerProject < 0 || row.Limits.People < 0 || row.Limits.IdleDays < 0 || row.Limits.LogDays < 0 {
		return fmt.Errorf("orgplan: %s: limits must not be negative", id)
	}
	return nil
}
