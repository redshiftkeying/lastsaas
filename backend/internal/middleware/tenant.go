package middleware

import (
	"context"
	"net/http"

	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	TenantContextKey     contextKey = "tenant"
	MembershipContextKey contextKey = "membership"
)

type TenantMiddleware struct {
	db *db.MongoDB
}

func NewTenantMiddleware(database *db.MongoDB) *TenantMiddleware {
	return &TenantMiddleware{db: database}
}

func (m *TenantMiddleware) RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If API key auth already populated tenant context, pass through
		if _, ok := GetTenantFromContext(r.Context()); ok {
			if _, ok := GetMembershipFromContext(r.Context()); ok {
				next.ServeHTTP(w, r)
				return
			}
		}

		tenantIDStr := r.Header.Get("X-Tenant-ID")
		if tenantIDStr == "" {
			http.Error(w, `{"error":"X-Tenant-ID header required"}`, http.StatusBadRequest)
			return
		}

		tenantID, err := primitive.ObjectIDFromHex(tenantIDStr)
		if err != nil {
			http.Error(w, `{"error":"Invalid tenant ID"}`, http.StatusBadRequest)
			return
		}

		var tenant models.Tenant
		err = m.db.Tenants().FindOne(r.Context(), bson.M{"_id": tenantID, "isActive": true}).Decode(&tenant)
		if err != nil {
			http.Error(w, `{"error":"Tenant not found"}`, http.StatusNotFound)
			return
		}

		user, ok := GetUserFromContext(r.Context())
		if !ok {
			http.Error(w, `{"error":"Not authenticated"}`, http.StatusUnauthorized)
			return
		}

		var membership models.TenantMembership
		err = m.db.TenantMemberships().FindOne(r.Context(), bson.M{
			"userId":   user.ID,
			"tenantId": tenantID,
		}).Decode(&membership)
		if err != nil {
			http.Error(w, `{"error":"Not a member of this tenant"}`, http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), TenantContextKey, &tenant)
		ctx = context.WithValue(ctx, MembershipContextKey, &membership)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func GetTenantFromContext(ctx context.Context) (*models.Tenant, bool) {
	tenant, ok := ctx.Value(TenantContextKey).(*models.Tenant)
	return tenant, ok
}

func GetMembershipFromContext(ctx context.Context) (*models.TenantMembership, bool) {
	membership, ok := ctx.Value(MembershipContextKey).(*models.TenantMembership)
	return membership, ok
}
