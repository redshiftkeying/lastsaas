package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UsageEvent struct {
	ID        primitive.ObjectID     `json:"id" bson:"_id,omitempty"`
	TenantID  primitive.ObjectID     `json:"tenantId" bson:"tenantId"`
	UserID    primitive.ObjectID     `json:"userId" bson:"userId"`
	Type      string                 `json:"type" bson:"type"`
	Quantity  int                    `json:"quantity" bson:"quantity"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" bson:"metadata,omitempty"`
	CreatedAt time.Time              `json:"createdAt" bson:"createdAt"`
}
