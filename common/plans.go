package common

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Plan struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	PriceCents   int64  `json:"priceCents"`
	Currency     string `json:"currency"`
	Interval     string `json:"interval"` // e.g., "month", "year"
	ChargeDomain bool   `json:"chargeDomain"`
	ProductId    string `json:"productId"`
	PriceId      string `json:"priceId"`
}

func LoadPlans(cfgDir string) ([]Plan, error) {
	buf, err := os.ReadFile(filepath.Join(cfgDir, "plans.json"))
	if err != nil {
		return nil, fmt.Errorf("failed to read plans.json: %w", err)
	}

	var plans []Plan
	if err := json.Unmarshal(buf, &plans); err != nil {
		return nil, fmt.Errorf("failed to parse plans.json: %w", err)
	}

	return plans, nil
}

func GetPlan(plans []Plan, planID string) *Plan {
	for _, plan := range plans {
		if plan.ID == planID {
			return &plan
		}
	}
	return nil
}
