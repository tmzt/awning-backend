package model

import (
	"strings"
)

// Onboarding types
// export type BusinessMotif = 'petStore' | 'gym' | 'tanning' | 'bakery' | 'generic';

// export type BusinessGoal =
//   | 'storeTraffic'      // Get people in my store
//   | 'serviceInfo'       // Give customers information on my services
//   | 'promotions'        // Plan and execute promotions for my store
//   | 'custom';           // Other, tell us

// export interface BusinessType {
//   id: string;
//   label: string;
//   category: string;
//   suggestedMotif?: BusinessMotif;
// }

// export type OnboardingDomainChoice = 'new' | 'existing' | 'skip' | null;

// export interface OnboardingData {
//   businessName: string;
//   businessType: BusinessType | null;
//   goals: BusinessGoal[];
//   customGoal?: string;
//   customNotes?: string;
//   domainChoice?: OnboardingDomainChoice;
//   domain?: string;
//   selectedMotif: BusinessMotif;
//   // canBuildSite?: boolean;
//   completed: boolean;
// }

type BusinessMotif string

const (
	BusinessMotifPetStore BusinessMotif = "petStore"
	BusinessMotifGym      BusinessMotif = "gym"
	BusinessMotifTanning  BusinessMotif = "tanning"
	BusinessMotifBakery   BusinessMotif = "bakery"
	BusinessMotifGeneric  BusinessMotif = "generic"
)

type BusinessGoal string

const (
	BusinessGoalStoreTraffic BusinessGoal = "storeTraffic"
	BusinessGoalServiceInfo  BusinessGoal = "serviceInfo"
	BusinessGoalPromotions   BusinessGoal = "promotions"
	BusinessGoalCustom       BusinessGoal = "custom"
)

type BusinessType string

type BusinessTypeData struct {
	ID             string         `json:"id"`
	Label          string         `json:"label"`
	Category       string         `json:"category"`
	SuggestedMotif *BusinessMotif `json:"suggestedMotif,omitempty"`
}

type OnboardingDomainChoice string

const (
	OnboardingDomainChoiceNew      OnboardingDomainChoice = "new"
	OnboardingDomainChoiceExisting OnboardingDomainChoice = "existing"
	OnboardingDomainChoiceSkip     OnboardingDomainChoice = "skip"
)

type OnboardingData struct {
	BusinessName     string                  `json:"businessName"`
	BusinessTypeData *BusinessTypeData       `json:"businessTypeData"`
	Goals            []BusinessGoal          `json:"goals"`
	CustomGoal       string                  `json:"customGoal,omitempty"`
	CustomNotes      string                  `json:"customNotes,omitempty"`
	DomainChoice     *OnboardingDomainChoice `json:"domainChoice,omitempty"`
	Domain           string                  `json:"domain,omitempty"`
	SelectedMotif    BusinessMotif           `json:"selectedMotif"`
	Completed        bool                    `json:"completed"`
}

// ToMap returns a map of template keys to string values, supporting explicit nested keys.
func (o *OnboardingData) ToMap() map[string]string {
	m := make(map[string]string)
	m["businessName"] = o.BusinessName
	m["selectedMotif"] = string(o.SelectedMotif)
	m["customGoal"] = o.CustomGoal
	m["customNotes"] = o.CustomNotes
	m["domain"] = o.Domain
	m["completed"] = "false"
	if o.Completed {
		m["completed"] = "true"
	}
	if o.DomainChoice != nil {
		m["domainChoice"] = string(*o.DomainChoice)
	} else {
		m["domainChoice"] = ""
	}
	// BusinessType nested fields
	if o.BusinessTypeData != nil {
		m["businessType.id"] = o.BusinessTypeData.ID
		m["businessType.label"] = o.BusinessTypeData.Label
		m["businessType.category"] = o.BusinessTypeData.Category
		if o.BusinessTypeData.SuggestedMotif != nil {
			m["businessType.suggestedMotif"] = string(*o.BusinessTypeData.SuggestedMotif)
		} else {
			m["businessType.suggestedMotif"] = ""
		}
	} else {
		m["businessType.id"] = ""
		m["businessType.label"] = ""
		m["businessType.category"] = ""
		m["businessType.suggestedMotif"] = ""
	}
	// Goals as comma-separated string
	if len(o.Goals) > 0 {
		var goals []string
		for _, g := range o.Goals {
			goals = append(goals, string(g))
		}
		m["goals"] = strings.Join(goals, ", ")
	} else {
		m["goals"] = ""
	}
	return m
}
