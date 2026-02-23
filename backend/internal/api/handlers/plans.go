package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"lastsaas/internal/configstore"
	"lastsaas/internal/db"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	stripeservice "lastsaas/internal/stripe"
	"lastsaas/internal/syslog"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/gorilla/mux"
)

type PlansHandler struct {
	db          *db.MongoDB
	syslog      *syslog.Logger
	configStore *configstore.Store
	stripe      *stripeservice.Service
}

func NewPlansHandler(database *db.MongoDB, sysLogger *syslog.Logger, cfgStore *configstore.Store, stripeSvc *stripeservice.Service) *PlansHandler {
	return &PlansHandler{
		db:          database,
		syslog:      sysLogger,
		configStore: cfgStore,
		stripe:      stripeSvc,
	}
}

// ListPlans returns all plans sorted by createdAt, enriched with subscriber counts.
func (h *PlansHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	opts := options.Find().SetSort(bson.D{{Key: "createdAt", Value: 1}})
	cursor, err := h.db.Plans().Find(ctx, bson.M{}, opts)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list plans")
		return
	}
	defer cursor.Close(ctx)

	var plans []models.Plan
	if err := cursor.All(ctx, &plans); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to decode plans")
		return
	}
	if plans == nil {
		plans = []models.Plan{}
	}

	// Build subscriber counts per plan
	subCounts := make(map[primitive.ObjectID]int)
	aggCursor, err := h.db.Tenants().Aggregate(ctx, mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"planId": bson.M{"$exists": true}}}},
		{{Key: "$group", Value: bson.M{"_id": "$planId", "count": bson.M{"$sum": 1}}}},
	})
	if err == nil {
		defer aggCursor.Close(ctx)
		for aggCursor.Next(ctx) {
			var row struct {
				ID    primitive.ObjectID `bson:"_id"`
				Count int               `bson:"count"`
			}
			if aggCursor.Decode(&row) == nil {
				subCounts[row.ID] = row.Count
			}
		}
	}

	type planWithStats struct {
		models.Plan
		SubscriberCount int `json:"subscriberCount"`
	}
	result := make([]planWithStats, len(plans))
	for i, p := range plans {
		result[i].Plan = p
		result[i].SubscriberCount = subCounts[p.ID]
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{"plans": result})
}

// GetPlan returns a single plan by ID.
func (h *PlansHandler) GetPlan(w http.ResponseWriter, r *http.Request) {
	planID, err := primitive.ObjectIDFromHex(mux.Vars(r)["planId"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid plan ID")
		return
	}

	var plan models.Plan
	if err := h.db.Plans().FindOne(r.Context(), bson.M{"_id": planID}).Decode(&plan); err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Plan not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to get plan")
		return
	}
	respondWithJSON(w, http.StatusOK, plan)
}

// ListEntitlementKeys returns unique entitlement keys, types, and descriptions across all plans.
func (h *PlansHandler) ListEntitlementKeys(w http.ResponseWriter, r *http.Request) {
	cursor, err := h.db.Plans().Find(r.Context(), bson.M{})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list plans")
		return
	}
	defer cursor.Close(r.Context())

	type KeyInfo struct {
		Key         string `json:"key"`
		Type        string `json:"type"`
		Description string `json:"description"`
	}

	type keyData struct {
		typ         string
		description string
	}
	keyMap := make(map[string]keyData)
	for cursor.Next(r.Context()) {
		var plan models.Plan
		if err := cursor.Decode(&plan); err != nil {
			continue
		}
		for k, v := range plan.Entitlements {
			existing, ok := keyMap[k]
			if !ok {
				keyMap[k] = keyData{typ: string(v.Type), description: v.Description}
			} else if existing.description == "" && v.Description != "" {
				existing.description = v.Description
				keyMap[k] = existing
			}
		}
	}

	keys := make([]KeyInfo, 0, len(keyMap))
	for k, d := range keyMap {
		keys = append(keys, KeyInfo{Key: k, Type: d.typ, Description: d.description})
	}
	respondWithJSON(w, http.StatusOK, map[string]interface{}{"keys": keys})
}

type planRequest struct {
	Name                 string                              `json:"name"`
	Description          string                              `json:"description"`
	MonthlyPriceCents    int64                               `json:"monthlyPriceCents"`
	AnnualDiscountPct    int                                 `json:"annualDiscountPct"`
	UsageCreditsPerMonth int64                               `json:"usageCreditsPerMonth"`
	CreditResetPolicy    string                              `json:"creditResetPolicy"`
	BonusCredits         int64                               `json:"bonusCredits"`
	UserLimit            int                                 `json:"userLimit"`
	Entitlements         map[string]models.EntitlementValue   `json:"entitlements"`
}

func validatePlanRequest(req *planRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		return fmt.Errorf("name is required")
	}
	if req.MonthlyPriceCents < 0 {
		return fmt.Errorf("monthly price must be >= 0")
	}
	if req.AnnualDiscountPct < 0 || req.AnnualDiscountPct > 100 {
		return fmt.Errorf("annual discount must be 0-100")
	}
	if req.UsageCreditsPerMonth < 0 {
		return fmt.Errorf("usage credits must be >= 0")
	}
	if req.CreditResetPolicy == "" {
		req.CreditResetPolicy = "reset"
	}
	if req.CreditResetPolicy != "reset" && req.CreditResetPolicy != "accrue" {
		return fmt.Errorf("credit reset policy must be 'reset' or 'accrue'")
	}
	if req.BonusCredits < 0 {
		return fmt.Errorf("bonus credits must be >= 0")
	}
	if req.UserLimit < 0 {
		return fmt.Errorf("user limit must be >= 0")
	}
	for k, v := range req.Entitlements {
		if v.Type != models.EntitlementTypeBool && v.Type != models.EntitlementTypeNumeric {
			return fmt.Errorf("entitlement %q has invalid type %q", k, v.Type)
		}
	}
	return nil
}

// CreatePlan creates a new plan.
func (h *PlansHandler) CreatePlan(w http.ResponseWriter, r *http.Request) {
	var req planRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := validatePlanRequest(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check name uniqueness
	count, _ := h.db.Plans().CountDocuments(r.Context(), bson.M{"name": req.Name})
	if count > 0 {
		respondWithError(w, http.StatusConflict, "A plan with this name already exists")
		return
	}

	entitlements := req.Entitlements
	if entitlements == nil {
		entitlements = map[string]models.EntitlementValue{}
	}

	now := time.Now()
	plan := models.Plan{
		Name:                 req.Name,
		Description:          strings.TrimSpace(req.Description),
		MonthlyPriceCents:    req.MonthlyPriceCents,
		AnnualDiscountPct:    req.AnnualDiscountPct,
		UsageCreditsPerMonth: req.UsageCreditsPerMonth,
		CreditResetPolicy:    models.CreditResetPolicy(req.CreditResetPolicy),
		BonusCredits:         req.BonusCredits,
		UserLimit:            req.UserLimit,
		Entitlements:         entitlements,
		IsSystem:             false,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	result, err := h.db.Plans().InsertOne(r.Context(), plan)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create plan")
		return
	}
	plan.ID = result.InsertedID.(primitive.ObjectID)

	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		h.syslog.LogWithUser(r.Context(), models.LogMedium, fmt.Sprintf("Plan created: %s", plan.Name), user.ID)
	}

	respondWithJSON(w, http.StatusCreated, plan)
}

// UpdatePlan updates an existing plan.
func (h *PlansHandler) UpdatePlan(w http.ResponseWriter, r *http.Request) {
	planID, err := primitive.ObjectIDFromHex(mux.Vars(r)["planId"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid plan ID")
		return
	}

	var existing models.Plan
	if err := h.db.Plans().FindOne(r.Context(), bson.M{"_id": planID}).Decode(&existing); err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Plan not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to get plan")
		return
	}

	var req planRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := validatePlanRequest(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, err.Error())
		return
	}

	// System plans can't have their name changed
	if existing.IsSystem && req.Name != existing.Name {
		respondWithError(w, http.StatusForbidden, "Cannot rename a system plan")
		return
	}

	// Check name uniqueness if name changed
	if req.Name != existing.Name {
		count, _ := h.db.Plans().CountDocuments(r.Context(), bson.M{"name": req.Name, "_id": bson.M{"$ne": planID}})
		if count > 0 {
			respondWithError(w, http.StatusConflict, "A plan with this name already exists")
			return
		}
	}

	entitlements := req.Entitlements
	if entitlements == nil {
		entitlements = map[string]models.EntitlementValue{}
	}

	// Invalidate Stripe price cache if pricing changed
	if req.MonthlyPriceCents != existing.MonthlyPriceCents || req.AnnualDiscountPct != existing.AnnualDiscountPct {
		h.db.StripeMappings().DeleteMany(r.Context(), bson.M{
			"entityType": bson.M{"$in": []string{"plan_month", "plan_year"}},
			"entityId":   planID,
		})
	}

	update := bson.M{"$set": bson.M{
		"name":                 req.Name,
		"description":          strings.TrimSpace(req.Description),
		"monthlyPriceCents":    req.MonthlyPriceCents,
		"annualDiscountPct":    req.AnnualDiscountPct,
		"usageCreditsPerMonth": req.UsageCreditsPerMonth,
		"creditResetPolicy":    req.CreditResetPolicy,
		"bonusCredits":         req.BonusCredits,
		"userLimit":            req.UserLimit,
		"entitlements":         entitlements,
		"updatedAt":            time.Now(),
	}}

	if _, err := h.db.Plans().UpdateByID(r.Context(), planID, update); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to update plan")
		return
	}

	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		h.syslog.LogWithUser(r.Context(), models.LogMedium, fmt.Sprintf("Plan updated: %s", req.Name), user.ID)
	}

	// Return updated plan with subscriber count
	var updated models.Plan
	h.db.Plans().FindOne(r.Context(), bson.M{"_id": planID}).Decode(&updated)
	subCount, _ := h.db.Tenants().CountDocuments(r.Context(), bson.M{"planId": planID})
	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   updated.ID,
		"name":                 updated.Name,
		"description":          updated.Description,
		"monthlyPriceCents":    updated.MonthlyPriceCents,
		"annualDiscountPct":    updated.AnnualDiscountPct,
		"usageCreditsPerMonth": updated.UsageCreditsPerMonth,
		"creditResetPolicy":    updated.CreditResetPolicy,
		"bonusCredits":         updated.BonusCredits,
		"userLimit":            updated.UserLimit,
		"entitlements":         updated.Entitlements,
		"isSystem":             updated.IsSystem,
		"isArchived":           updated.IsArchived,
		"createdAt":            updated.CreatedAt,
		"updatedAt":            updated.UpdatedAt,
		"subscriberCount":      int(subCount),
	})
}

// DeletePlan deletes a non-system plan if no tenants use it.
func (h *PlansHandler) DeletePlan(w http.ResponseWriter, r *http.Request) {
	planID, err := primitive.ObjectIDFromHex(mux.Vars(r)["planId"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid plan ID")
		return
	}

	var plan models.Plan
	if err := h.db.Plans().FindOne(r.Context(), bson.M{"_id": planID}).Decode(&plan); err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Plan not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to get plan")
		return
	}

	if plan.IsSystem {
		respondWithError(w, http.StatusForbidden, "Cannot delete a system plan")
		return
	}

	tenantCount, _ := h.db.Tenants().CountDocuments(r.Context(), bson.M{"planId": planID})
	if tenantCount > 0 {
		respondWithError(w, http.StatusConflict, fmt.Sprintf("Cannot delete plan: %d tenant(s) are using it", tenantCount))
		return
	}

	if _, err := h.db.Plans().DeleteOne(r.Context(), bson.M{"_id": planID}); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to delete plan")
		return
	}

	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		h.syslog.LogWithUser(r.Context(), models.LogMedium, fmt.Sprintf("Plan deleted: %s", plan.Name), user.ID)
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// ArchivePlan sets isArchived=true, hiding the plan from new subscribers.
func (h *PlansHandler) ArchivePlan(w http.ResponseWriter, r *http.Request) {
	planID, err := primitive.ObjectIDFromHex(mux.Vars(r)["planId"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid plan ID")
		return
	}

	var plan models.Plan
	if err := h.db.Plans().FindOne(r.Context(), bson.M{"_id": planID}).Decode(&plan); err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Plan not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to get plan")
		return
	}

	if plan.IsSystem {
		respondWithError(w, http.StatusForbidden, "Cannot archive a system plan")
		return
	}

	h.db.Plans().UpdateByID(r.Context(), planID, bson.M{"$set": bson.M{"isArchived": true, "updatedAt": time.Now()}})

	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		h.syslog.LogWithUser(r.Context(), models.LogMedium, fmt.Sprintf("Plan archived: %s", plan.Name), user.ID)
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// UnarchivePlan sets isArchived=false, making the plan visible again.
func (h *PlansHandler) UnarchivePlan(w http.ResponseWriter, r *http.Request) {
	planID, err := primitive.ObjectIDFromHex(mux.Vars(r)["planId"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid plan ID")
		return
	}

	var plan models.Plan
	if err := h.db.Plans().FindOne(r.Context(), bson.M{"_id": planID}).Decode(&plan); err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Plan not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to get plan")
		return
	}

	if plan.IsSystem {
		respondWithError(w, http.StatusForbidden, "Cannot unarchive a system plan")
		return
	}

	h.db.Plans().UpdateByID(r.Context(), planID, bson.M{"$set": bson.M{"isArchived": false, "updatedAt": time.Now()}})

	if user, ok := middleware.GetUserFromContext(r.Context()); ok {
		h.syslog.LogWithUser(r.Context(), models.LogMedium, fmt.Sprintf("Plan unarchived: %s", plan.Name), user.ID)
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"status": "unarchived"})
}

// AssignPlan sets a tenant's plan and/or billing waived status.
// Handles billing edge cases:
//   - Waiving billing on a tenant with an active Stripe subscription cancels the subscription.
//   - Removing the billing waiver from a tenant on a paid plan (with no subscription) downgrades to the system plan.
//   - Assigning a paid plan to a tenant without waiving billing is rejected (admin must either waive or let the user subscribe).
func (h *PlansHandler) AssignPlan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tenantID, err := primitive.ObjectIDFromHex(mux.Vars(r)["tenantId"])
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	var req struct {
		PlanID        *string `json:"planId"`
		BillingWaived *bool   `json:"billingWaived,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Verify tenant exists
	var tenant models.Tenant
	if err := h.db.Tenants().FindOne(ctx, bson.M{"_id": tenantID}).Decode(&tenant); err != nil {
		if err == mongo.ErrNoDocuments {
			respondWithError(w, http.StatusNotFound, "Tenant not found")
			return
		}
		respondWithError(w, http.StatusInternalServerError, "Failed to get tenant")
		return
	}

	setFields := bson.M{"updatedAt": time.Now()}
	unsetFields := bson.M{}
	var planName string
	var newPlan *models.Plan

	if req.PlanID != nil {
		if *req.PlanID != "" {
			planOID, err := primitive.ObjectIDFromHex(*req.PlanID)
			if err != nil {
				respondWithError(w, http.StatusBadRequest, "Invalid plan ID")
				return
			}
			var plan models.Plan
			if err := h.db.Plans().FindOne(ctx, bson.M{"_id": planOID}).Decode(&plan); err != nil {
				if err == mongo.ErrNoDocuments {
					respondWithError(w, http.StatusNotFound, "Plan not found")
					return
				}
				respondWithError(w, http.StatusInternalServerError, "Failed to get plan")
				return
			}
			newPlan = &plan
			planName = plan.Name
			setFields["planId"] = planOID
		} else {
			planName = "(system default)"
			unsetFields["planId"] = ""
		}
	}

	// Determine the effective billing waived state after this update
	effectiveWaived := tenant.BillingWaived
	if req.BillingWaived != nil {
		effectiveWaived = *req.BillingWaived
		setFields["billingWaived"] = *req.BillingWaived
	}

	// Determine the effective plan after this update
	effectivePlan := newPlan
	if effectivePlan == nil && tenant.PlanID != nil && (req.PlanID == nil || *req.PlanID != "") {
		// Plan isn't changing — look up the current plan for price checks
		var currentPlan models.Plan
		if err := h.db.Plans().FindOne(ctx, bson.M{"_id": *tenant.PlanID}).Decode(&currentPlan); err == nil {
			effectivePlan = &currentPlan
		}
	}

	// Edge case: assigning a paid plan without billing waived and no active subscription
	if effectivePlan != nil && effectivePlan.MonthlyPriceCents > 0 && !effectiveWaived && tenant.StripeSubscriptionID == "" {
		respondWithError(w, http.StatusBadRequest, "Cannot assign a paid plan without waiving billing. The tenant has no active subscription to cover the cost. Either waive billing or let the tenant subscribe through the checkout flow.")
		return
	}

	// Edge case: removing billing waiver from a tenant on a paid plan with no subscription
	if req.BillingWaived != nil && !*req.BillingWaived && tenant.BillingWaived {
		if effectivePlan != nil && effectivePlan.MonthlyPriceCents > 0 && tenant.StripeSubscriptionID == "" {
			// Auto-downgrade: remove planId so they fall back to the system default plan
			unsetFields["planId"] = ""
			delete(setFields, "planId")
			planName = "(system default — billing waiver removed)"
		}
	}

	// Edge case: waiving billing while tenant has an active Stripe subscription — cancel it
	if req.BillingWaived != nil && *req.BillingWaived && !tenant.BillingWaived {
		if tenant.StripeSubscriptionID != "" && (tenant.BillingStatus == models.BillingStatusActive || tenant.BillingStatus == models.BillingStatusPastDue) {
			if h.stripe != nil {
				if err := h.stripe.CancelSubscriptionImmediately(ctx, tenant.StripeSubscriptionID); err != nil {
					log.Printf("AssignPlan: failed to cancel subscription for tenant %s: %v", tenant.Name, err)
					respondWithError(w, http.StatusInternalServerError, "Failed to cancel existing subscription")
					return
				}
			}
			setFields["stripeSubscriptionId"] = ""
			setFields["billingStatus"] = models.BillingStatusNone
			setFields["billingInterval"] = ""
			unsetFields["currentPeriodEnd"] = ""
			unsetFields["canceledAt"] = ""
		}
	}

	update := bson.M{"$set": setFields}
	if len(unsetFields) > 0 {
		update["$unset"] = unsetFields
	}

	if _, err := h.db.Tenants().UpdateByID(ctx, tenantID, update); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to assign plan")
		return
	}

	if user, ok := middleware.GetUserFromContext(ctx); ok {
		if planName != "" {
			h.syslog.LogWithUser(ctx, models.LogMedium, fmt.Sprintf("Tenant %s assigned to plan: %s", tenant.Name, planName), user.ID)
		}
		if req.BillingWaived != nil {
			h.syslog.LogWithUser(ctx, models.LogMedium, fmt.Sprintf("Tenant %s billing waived: %v", tenant.Name, *req.BillingWaived), user.ID)
		}
	}

	respondWithJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// ListPlansPublic returns all plans for authenticated users along with their tenant's current plan.
func (h *PlansHandler) ListPlansPublic(w http.ResponseWriter, r *http.Request) {
	// Get tenant from X-Tenant-ID header
	tenantIDStr := r.Header.Get("X-Tenant-ID")
	if tenantIDStr == "" {
		respondWithError(w, http.StatusBadRequest, "Tenant ID required")
		return
	}
	tenantID, err := primitive.ObjectIDFromHex(tenantIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid tenant ID")
		return
	}

	var tenant models.Tenant
	if err := h.db.Tenants().FindOne(r.Context(), bson.M{"_id": tenantID}).Decode(&tenant); err != nil {
		respondWithError(w, http.StatusNotFound, "Tenant not found")
		return
	}

	// Get non-archived plans sorted by price then name
	// If the tenant's current plan is archived, include it too
	filter := bson.M{"isArchived": bson.M{"$ne": true}}
	if tenant.PlanID != nil {
		filter = bson.M{"$or": []bson.M{
			{"isArchived": bson.M{"$ne": true}},
			{"_id": *tenant.PlanID},
		}}
	}
	opts := options.Find().SetSort(bson.D{{Key: "monthlyPriceCents", Value: 1}, {Key: "name", Value: 1}})
	cursor, err := h.db.Plans().Find(r.Context(), filter, opts)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to list plans")
		return
	}
	defer cursor.Close(r.Context())

	var plans []models.Plan
	if err := cursor.All(r.Context(), &plans); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to decode plans")
		return
	}
	if plans == nil {
		plans = []models.Plan{}
	}

	// Resolve current plan and compute limits
	currentPlanID := ""
	currentPlanUserLimit := 0
	var currentPlan *models.Plan
	if tenant.PlanID != nil {
		currentPlanID = tenant.PlanID.Hex()
		for i := range plans {
			if plans[i].ID == *tenant.PlanID {
				currentPlan = &plans[i]
				break
			}
		}
	}
	if currentPlan == nil {
		// Fall back to system plan
		for i := range plans {
			if plans[i].IsSystem {
				currentPlanID = plans[i].ID.Hex()
				currentPlan = &plans[i]
				break
			}
		}
	}
	if currentPlan != nil {
		currentPlanUserLimit = currentPlan.UserLimit
	}

	// Compute max user limit across all visible plans (0 = at least one plan is unlimited)
	maxPlanUserLimit := 1
	for _, p := range plans {
		if p.UserLimit == 0 {
			maxPlanUserLimit = 0
			break
		}
		if p.UserLimit > maxPlanUserLimit {
			maxPlanUserLimit = p.UserLimit
		}
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{
		"plans":                      plans,
		"currentPlanId":              currentPlanID,
		"billingWaived":              tenant.BillingWaived,
		"tenantSubscriptionCredits":  tenant.SubscriptionCredits,
		"tenantPurchasedCredits":     tenant.PurchasedCredits,
		"billingStatus":              tenant.BillingStatus,
		"billingInterval":            tenant.BillingInterval,
		"currentPeriodEnd":           tenant.CurrentPeriodEnd,
		"canceledAt":                 tenant.CanceledAt,
		"currentPlanUserLimit":       currentPlanUserLimit,
		"maxPlanUserLimit":           maxPlanUserLimit,
		"upgradePromptTitle":                h.configStore.Get("team.upgrade_prompt.title"),
		"upgradePromptBody":                 h.configStore.Get("team.upgrade_prompt.body"),
		"entitlementUpgradePromptTitle":       h.configStore.Get("entitlement.upgrade_prompt.title"),
		"entitlementUpgradePromptBody":        h.configStore.Get("entitlement.upgrade_prompt.body"),
		"entitlementUpgradePromptNumericBody": h.configStore.Get("entitlement.upgrade_prompt.numeric_body"),
	})
}

// lookupPlanForTenant returns the plan for a tenant, falling back to the system free plan.
func (h *PlansHandler) lookupPlanForTenant(ctx context.Context, tenant *models.Tenant) (*models.Plan, error) {
	var plan models.Plan
	if tenant.PlanID != nil {
		err := h.db.Plans().FindOne(ctx, bson.M{"_id": *tenant.PlanID}).Decode(&plan)
		return &plan, err
	}
	err := h.db.Plans().FindOne(ctx, bson.M{"isSystem": true}).Decode(&plan)
	return &plan, err
}
