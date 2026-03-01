package metrics

import (
	"context"
	"log/slog"
	"os"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	lockName    = "metrics_leader"
	leaseTTL    = 2 * time.Minute
	renewalTick = 30 * time.Second
	collectTick = 1 * time.Hour
)

type Service struct {
	db       *db.MongoDB
	holderID string
	stop     chan struct{}
}

func New(database *db.MongoDB) *Service {
	// Use hostname + PID as a unique holder ID per machine
	hostname, _ := os.Hostname()
	holderID := hostname + "-" + time.Now().Format("20060102150405")

	return &Service{
		db:       database,
		holderID: holderID,
		stop:     make(chan struct{}),
	}
}

func (s *Service) Start() {
	go s.run()
	slog.Info("Daily metrics service started", "holder", s.holderID)
}

func (s *Service) Stop() {
	close(s.stop)
	// Release the lock on shutdown so another machine can take over immediately
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	s.releaseLock(ctx)
}

func (s *Service) run() {
	// Try to acquire leadership immediately, then collect if we got it
	if s.tryAcquireOrRenew() {
		s.collectDaily()
	}

	renewTicker := time.NewTicker(renewalTick)
	collectTicker := time.NewTicker(collectTick)
	defer renewTicker.Stop()
	defer collectTicker.Stop()

	for {
		select {
		case <-renewTicker.C:
			s.tryAcquireOrRenew()
		case <-collectTicker.C:
			if s.isLeader() {
				s.collectDaily()
			}
		case <-s.stop:
			return
		}
	}
}

// tryAcquireOrRenew attempts to claim or renew the leader lock.
// Returns true if this instance is the leader after the call.
func (s *Service) tryAcquireOrRenew() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC()
	newExpiry := now.Add(leaseTTL)

	// Try to upsert: either claim an expired/missing lock, or renew our own
	filter := bson.M{
		"_id": lockName,
		"$or": bson.A{
			bson.M{"holderId": s.holderID},           // we already hold it
			bson.M{"expiresAt": bson.M{"$lte": now}}, // expired, anyone can claim
		},
	}
	update := bson.M{
		"$set": bson.M{
			"holderId":  s.holderID,
			"expiresAt": newExpiry,
			"updatedAt": now,
		},
		"$setOnInsert": bson.M{
			"_id":       lockName,
			"createdAt": now,
		},
	}

	result := s.db.LeaderLocks().FindOneAndUpdate(ctx, filter, update,
		options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After),
	)

	if result.Err() != nil {
		if result.Err() == mongo.ErrNoDocuments {
			// Another holder has the lock and it hasn't expired
			return false
		}
		// On upsert conflict (duplicate key during race), the other machine won
		if mongo.IsDuplicateKeyError(result.Err()) {
			return false
		}
		slog.Error("Metrics leader lock error", "error", result.Err())
		return false
	}

	var doc struct {
		HolderID string `bson:"holderId"`
	}
	if err := result.Decode(&doc); err != nil {
		return false
	}
	return doc.HolderID == s.holderID
}

// isLeader checks if this instance currently holds the lock.
func (s *Service) isLeader() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var doc struct {
		HolderID  string    `bson:"holderId"`
		ExpiresAt time.Time `bson:"expiresAt"`
	}
	err := s.db.LeaderLocks().FindOne(ctx, bson.M{"_id": lockName}).Decode(&doc)
	if err != nil {
		return false
	}
	return doc.HolderID == s.holderID && doc.ExpiresAt.After(time.Now().UTC())
}

// releaseLock removes the lock if we hold it, so another machine can take over.
func (s *Service) releaseLock(ctx context.Context) {
	_, _ = s.db.LeaderLocks().DeleteOne(ctx, bson.M{
		"_id":      lockName,
		"holderId": s.holderID,
	})
}

func (s *Service) collectDaily() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	now := time.Now().UTC()
	dateStr := now.Format("2006-01-02")

	// DAU + MAU in a single aggregation pipeline
	dayAgo := now.Add(-24 * time.Hour)
	monthAgo := now.AddDate(0, 0, -30)

	dauMauPipeline := bson.A{
		bson.M{"$match": bson.M{"lastLoginAt": bson.M{"$gte": monthAgo}}},
		bson.M{"$group": bson.M{
			"_id": nil,
			"mau": bson.M{"$sum": 1},
			"dau": bson.M{"$sum": bson.M{"$cond": bson.A{
				bson.M{"$gte": bson.A{"$lastLoginAt", dayAgo}}, 1, 0,
			}}},
		}},
	}
	var dauCount, mauCount int64
	dauMauCursor, err := s.db.Users().Aggregate(ctx, dauMauPipeline)
	if err != nil {
		slog.Error("Metrics DAU/MAU aggregation error", "error", err)
	} else {
		defer dauMauCursor.Close(ctx)
		var results []struct {
			DAU int64 `bson:"dau"`
			MAU int64 `bson:"mau"`
		}
		if dauMauCursor.All(ctx, &results) == nil && len(results) > 0 {
			dauCount = results[0].DAU
			mauCount = results[0].MAU
		}
	}

	// Revenue today: sum amountCents from financial_transactions created today
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	dayEnd := dayStart.Add(24 * time.Hour)

	revPipeline := bson.A{
		bson.M{"$match": bson.M{
			"createdAt": bson.M{"$gte": dayStart, "$lt": dayEnd},
		}},
		bson.M{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$amountCents"},
		}},
	}
	revCursor, err := s.db.FinancialTransactions().Aggregate(ctx, revPipeline)
	var revenue int64
	if err == nil {
		defer revCursor.Close(ctx)
		var result []struct {
			Total int64 `bson:"total"`
		}
		if revCursor.All(ctx, &result) == nil && len(result) > 0 {
			revenue = result[0].Total
		}
	}

	// ARR: sum monthly price * 12 for all active subscriptions
	arrPipeline := bson.A{
		bson.M{"$match": bson.M{
			"billingStatus": models.BillingStatusActive,
			"planId":        bson.M{"$ne": nil},
		}},
		bson.M{"$lookup": bson.M{
			"from":         "plans",
			"localField":   "planId",
			"foreignField": "_id",
			"as":           "plan",
		}},
		bson.M{"$unwind": bson.M{"path": "$plan", "preserveNullAndEmptyArrays": false}},
		bson.M{"$group": bson.M{
			"_id":               nil,
			"totalMonthlyCents": bson.M{"$sum": "$plan.monthlyPriceCents"},
		}},
	}
	arrCursor, err := s.db.Tenants().Aggregate(ctx, arrPipeline)
	var arr int64
	if err == nil {
		defer arrCursor.Close(ctx)
		var result []struct {
			TotalMonthlyCents int64 `bson:"totalMonthlyCents"`
		}
		if arrCursor.All(ctx, &result) == nil && len(result) > 0 {
			arr = result[0].TotalMonthlyCents * 12
		}
	}

	// Upsert daily metric
	_, err = s.db.DailyMetrics().UpdateOne(ctx,
		bson.M{"date": dateStr},
		bson.M{"$set": bson.M{
			"dau":       dauCount,
			"mau":       mauCount,
			"revenue":   revenue,
			"arr":       arr,
			"createdAt": now,
		}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		slog.Error("Metrics upsert daily metric error", "error", err)
	}
}
