package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/email"
	"lastsaas/internal/events"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/syslog"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"

	"github.com/gorilla/mux"
)

type TenantHandler struct {
	db           *db.MongoDB
	emailService *email.ResendService
	events       events.Emitter
	syslog       *syslog.Logger
}

func NewTenantHandler(database *db.MongoDB, emailService *email.ResendService, emitter events.Emitter, sysLogger *syslog.Logger) *TenantHandler {
	return &TenantHandler{
		db:           database,
		emailService: emailService,
		events:       emitter,
		syslog:       sysLogger,
	}
}

// --- Request types ---

type InviteMemberRequest struct {
	Email string            `json:"email"`
	Role  models.MemberRole `json:"role"`
}

type ChangeRoleRequest struct {
	Role models.MemberRole `json:"role"`
}

type MemberResponse struct {
	UserID      string            `json:"userId"`
	Email       string            `json:"email"`
	DisplayName string            `json:"displayName"`
	Role        models.MemberRole `json:"role"`
	JoinedAt    time.Time         `json:"joinedAt"`
}

// --- Handlers ---

func (h *TenantHandler) ListMembers(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Tenant context missing")
		return
	}

	cursor, err := h.db.TenantMemberships().Find(r.Context(), bson.M{"tenantId": tenant.ID})
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to fetch members")
		return
	}
	defer cursor.Close(r.Context())

	var memberships []models.TenantMembership
	if err := cursor.All(r.Context(), &memberships); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to decode members")
		return
	}

	var members []MemberResponse
	for _, m := range memberships {
		var user models.User
		if err := h.db.Users().FindOne(r.Context(), bson.M{"_id": m.UserID}).Decode(&user); err != nil {
			continue
		}
		members = append(members, MemberResponse{
			UserID:      user.ID.Hex(),
			Email:       user.Email,
			DisplayName: user.DisplayName,
			Role:        m.Role,
			JoinedAt:    m.JoinedAt,
		})
	}

	respondWithJSON(w, http.StatusOK, map[string]interface{}{"members": members})
}

func (h *TenantHandler) InviteMember(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Tenant context missing")
		return
	}
	membership, ok := middleware.GetMembershipFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusForbidden, "Membership context missing")
		return
	}
	user, ok := middleware.GetUserFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusUnauthorized, "Not authenticated")
		return
	}

	var req InviteMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		respondWithError(w, http.StatusBadRequest, "Email is required")
		return
	}

	// Validate role
	if req.Role == models.RoleOwner {
		respondWithError(w, http.StatusBadRequest, "Cannot invite as owner. Use transfer ownership instead.")
		return
	}
	if !models.ValidRole(req.Role) {
		respondWithError(w, http.StatusBadRequest, "Invalid role")
		return
	}

	// Permission check: admins can invite users, owners can invite admins
	if req.Role == models.RoleAdmin && membership.Role != models.RoleOwner {
		respondWithError(w, http.StatusForbidden, "Only owners can invite admins")
		return
	}

	// Check if user is already a member
	var existingUser models.User
	if err := h.db.Users().FindOne(r.Context(), bson.M{"email": req.Email}).Decode(&existingUser); err == nil {
		count, _ := h.db.TenantMemberships().CountDocuments(r.Context(), bson.M{
			"userId":   existingUser.ID,
			"tenantId": tenant.ID,
		})
		if count > 0 {
			respondWithError(w, http.StatusConflict, "User is already a member of this tenant")
			return
		}
	}

	// Check if invitation already exists
	count, _ := h.db.Invitations().CountDocuments(r.Context(), bson.M{
		"tenantId":  tenant.ID,
		"email":     req.Email,
		"status":    models.InvitationPending,
		"expiresAt": bson.M{"$gt": time.Now()},
	})
	if count > 0 {
		respondWithError(w, http.StatusConflict, "An invitation has already been sent to this email")
		return
	}

	// Enforce plan user limit
	var tenantPlan models.Plan
	if tenant.PlanID != nil {
		h.db.Plans().FindOne(r.Context(), bson.M{"_id": *tenant.PlanID}).Decode(&tenantPlan)
	} else {
		h.db.Plans().FindOne(r.Context(), bson.M{"isSystem": true}).Decode(&tenantPlan)
	}
	if tenantPlan.UserLimit > 0 {
		memberCount, _ := h.db.TenantMemberships().CountDocuments(r.Context(), bson.M{"tenantId": tenant.ID})
		pendingCount, _ := h.db.Invitations().CountDocuments(r.Context(), bson.M{
			"tenantId":  tenant.ID,
			"status":    models.InvitationPending,
			"expiresAt": bson.M{"$gt": time.Now()},
		})
		if memberCount+pendingCount >= int64(tenantPlan.UserLimit) {
			respondWithJSON(w, http.StatusForbidden, map[string]interface{}{
				"error":     fmt.Sprintf("User limit reached. Your plan allows %d users.", tenantPlan.UserLimit),
				"code":      "USER_LIMIT_REACHED",
				"userLimit": tenantPlan.UserLimit,
			})
			return
		}
	}

	now := time.Now()
	token := generateRandomToken()
	invitation := models.Invitation{
		ID:        primitive.NewObjectID(),
		TenantID:  tenant.ID,
		Email:     req.Email,
		Role:      req.Role,
		Token:     token,
		Status:    models.InvitationPending,
		InvitedBy: user.ID,
		ExpiresAt: now.Add(7 * 24 * time.Hour),
		CreatedAt: now,
	}

	if _, err := h.db.Invitations().InsertOne(r.Context(), invitation); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to create invitation")
		return
	}

	// Send invitation email
	go func() {
		if h.emailService != nil {
			if err := h.emailService.SendInvitationEmail(req.Email, user.DisplayName, tenant.Name, token); err != nil {
				log.Printf("Failed to send invitation email to %s: %v", req.Email, err)
			}
		}
	}()

	h.events.Emit(events.Event{
		Type:      events.EventMemberInvited,
		Timestamp: now,
		Data: map[string]interface{}{
			"tenantId": tenant.ID.Hex(),
			"email":    req.Email,
			"role":     string(req.Role),
		},
	})

	respondWithJSON(w, http.StatusCreated, map[string]string{"message": "Invitation sent"})
}

func (h *TenantHandler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Tenant context missing")
		return
	}
	currentMembership, ok := middleware.GetMembershipFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusForbidden, "Membership context missing")
		return
	}

	targetUserIDStr := mux.Vars(r)["userId"]
	targetUserID, err := primitive.ObjectIDFromHex(targetUserIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Cannot remove yourself
	if targetUserID == currentMembership.UserID {
		respondWithError(w, http.StatusBadRequest, "Cannot remove yourself. Leave the tenant instead.")
		return
	}

	// Find target membership
	var targetMembership models.TenantMembership
	if err := h.db.TenantMemberships().FindOne(r.Context(), bson.M{
		"userId":   targetUserID,
		"tenantId": tenant.ID,
	}).Decode(&targetMembership); err != nil {
		respondWithError(w, http.StatusNotFound, "Member not found")
		return
	}

	// Cannot remove the owner
	if targetMembership.Role == models.RoleOwner {
		respondWithError(w, http.StatusForbidden, "Cannot remove the owner. Transfer ownership first.")
		return
	}

	// Admins can only remove users
	if currentMembership.Role == models.RoleAdmin && targetMembership.Role != models.RoleUser {
		respondWithError(w, http.StatusForbidden, "Admins can only remove users")
		return
	}

	if _, err := h.db.TenantMemberships().DeleteOne(r.Context(), bson.M{"_id": targetMembership.ID}); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Failed to remove member")
		return
	}

	h.syslog.High(r.Context(), fmt.Sprintf("Member removed: user %s from tenant %s (%s)", targetUserID.Hex(), tenant.Name, tenant.ID.Hex()))

	h.events.Emit(events.Event{
		Type:      events.EventMemberRemoved,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"tenantId": tenant.ID.Hex(),
			"userId":   targetUserID.Hex(),
		},
	})

	respondWithJSON(w, http.StatusOK, map[string]string{"message": "Member removed"})
}

func (h *TenantHandler) ChangeRole(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Tenant context missing")
		return
	}
	currentMembership, ok := middleware.GetMembershipFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusForbidden, "Membership context missing")
		return
	}

	// Only owner can change roles
	if currentMembership.Role != models.RoleOwner {
		respondWithError(w, http.StatusForbidden, "Only the owner can change roles")
		return
	}

	targetUserIDStr := mux.Vars(r)["userId"]
	targetUserID, err := primitive.ObjectIDFromHex(targetUserIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	// Cannot change own role
	if targetUserID == currentMembership.UserID {
		respondWithError(w, http.StatusBadRequest, "Cannot change your own role")
		return
	}

	var req ChangeRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Role == models.RoleOwner {
		respondWithError(w, http.StatusBadRequest, "Cannot set role to owner. Use transfer ownership instead.")
		return
	}
	if !models.ValidRole(req.Role) {
		respondWithError(w, http.StatusBadRequest, "Invalid role")
		return
	}

	result, err := h.db.TenantMemberships().UpdateOne(r.Context(),
		bson.M{"userId": targetUserID, "tenantId": tenant.ID},
		bson.M{"$set": bson.M{"role": req.Role, "updatedAt": time.Now()}},
	)
	if err != nil || result.MatchedCount == 0 {
		respondWithError(w, http.StatusNotFound, "Member not found")
		return
	}

	h.events.Emit(events.Event{
		Type:      events.EventMemberRoleChanged,
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"tenantId": tenant.ID.Hex(),
			"userId":   targetUserID.Hex(),
			"newRole":  string(req.Role),
		},
	})

	respondWithJSON(w, http.StatusOK, map[string]string{"message": "Role updated"})
}

func (h *TenantHandler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	tenant, ok := middleware.GetTenantFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusBadRequest, "Tenant context missing")
		return
	}
	currentMembership, ok := middleware.GetMembershipFromContext(r.Context())
	if !ok {
		respondWithError(w, http.StatusForbidden, "Membership context missing")
		return
	}

	if currentMembership.Role != models.RoleOwner {
		respondWithError(w, http.StatusForbidden, "Only the owner can transfer ownership")
		return
	}

	targetUserIDStr := mux.Vars(r)["userId"]
	targetUserID, err := primitive.ObjectIDFromHex(targetUserIDStr)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid user ID")
		return
	}

	if targetUserID == currentMembership.UserID {
		respondWithError(w, http.StatusBadRequest, "Cannot transfer ownership to yourself")
		return
	}

	// Verify target is a member
	count, _ := h.db.TenantMemberships().CountDocuments(r.Context(), bson.M{
		"userId":   targetUserID,
		"tenantId": tenant.ID,
	})
	if count == 0 {
		respondWithError(w, http.StatusNotFound, "Target user is not a member of this tenant")
		return
	}

	now := time.Now()

	// Set target as owner
	h.db.TenantMemberships().UpdateOne(r.Context(),
		bson.M{"userId": targetUserID, "tenantId": tenant.ID},
		bson.M{"$set": bson.M{"role": models.RoleOwner, "updatedAt": now}},
	)

	// Demote current owner to admin
	h.db.TenantMemberships().UpdateOne(r.Context(),
		bson.M{"userId": currentMembership.UserID, "tenantId": tenant.ID},
		bson.M{"$set": bson.M{"role": models.RoleAdmin, "updatedAt": now}},
	)

	h.events.Emit(events.Event{
		Type:      events.EventOwnershipTransferred,
		Timestamp: now,
		Data: map[string]interface{}{
			"tenantId":    tenant.ID.Hex(),
			"fromUserId":  currentMembership.UserID.Hex(),
			"toUserId":    targetUserID.Hex(),
		},
	})

	respondWithJSON(w, http.StatusOK, map[string]string{"message": "Ownership transferred"})
}
