package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type EntitlementType string

const (
	EntitlementTypeBool    EntitlementType = "bool"
	EntitlementTypeNumeric EntitlementType = "numeric"
)

type CreditResetPolicy string

const (
	CreditResetPolicyReset  CreditResetPolicy = "reset"
	CreditResetPolicyAccrue CreditResetPolicy = "accrue"
)

type EntitlementValue struct {
	Type         EntitlementType `json:"type" bson:"type"`
	BoolValue    bool            `json:"boolValue" bson:"boolValue"`
	NumericValue int64           `json:"numericValue" bson:"numericValue"`
	Description  string          `json:"description" bson:"description"`
}

type Plan struct {
	ID                   primitive.ObjectID          `json:"id" bson:"_id,omitempty"`
	Name                 string                      `json:"name" bson:"name"`
	Description          string                      `json:"description" bson:"description"`
	MonthlyPriceCents    int64                       `json:"monthlyPriceCents" bson:"monthlyPriceCents"`
	AnnualDiscountPct    int                         `json:"annualDiscountPct" bson:"annualDiscountPct"`
	UsageCreditsPerMonth int64                       `json:"usageCreditsPerMonth" bson:"usageCreditsPerMonth"`
	CreditResetPolicy    CreditResetPolicy           `json:"creditResetPolicy" bson:"creditResetPolicy"`
	BonusCredits         int64                       `json:"bonusCredits" bson:"bonusCredits"`
	UserLimit            int                         `json:"userLimit" bson:"userLimit"`
	Entitlements         map[string]EntitlementValue `json:"entitlements" bson:"entitlements"`
	IsSystem             bool                        `json:"isSystem" bson:"isSystem"`
	IsArchived           bool                        `json:"isArchived" bson:"isArchived"`
	CreatedAt            time.Time                   `json:"createdAt" bson:"createdAt"`
	UpdatedAt            time.Time                   `json:"updatedAt" bson:"updatedAt"`
}
