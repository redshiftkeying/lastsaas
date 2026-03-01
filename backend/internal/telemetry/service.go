package telemetry

import (
	"context"
	"log/slog"
	"math"
	"sort"
	"time"

	"lastsaas/internal/db"
	"lastsaas/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Service provides telemetry tracking and querying for product analytics.
// Apps using LastSaaS as a library can call Track/TrackBatch directly
// without going through the HTTP API.
type Service struct {
	db *db.MongoDB
}

// New creates a new telemetry service.
func New(database *db.MongoDB) *Service {
	return &Service{db: database}
}

// --- Tracking (Go SDK) ---

// Track records a single telemetry event.
func (s *Service) Track(ctx context.Context, event models.TelemetryEvent) error {
	if event.ID.IsZero() {
		event.ID = primitive.NewObjectID()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	_, err := s.db.TelemetryEvents().InsertOne(ctx, event)
	if err != nil {
		slog.Warn("telemetry: failed to track event", "event", event.EventName, "error", err)
	}
	return err
}

// TrackBatch records multiple events.
func (s *Service) TrackBatch(ctx context.Context, events []models.TelemetryEvent) error {
	if len(events) == 0 {
		return nil
	}
	docs := make([]interface{}, len(events))
	now := time.Now()
	for i := range events {
		if events[i].ID.IsZero() {
			events[i].ID = primitive.NewObjectID()
		}
		if events[i].CreatedAt.IsZero() {
			events[i].CreatedAt = now
		}
		docs[i] = events[i]
	}
	_, err := s.db.TelemetryEvents().InsertMany(ctx, docs)
	if err != nil {
		slog.Warn("telemetry: failed to track batch", "count", len(events), "error", err)
	}
	return err
}

// TrackPageView is a convenience method for page view events.
func (s *Service) TrackPageView(ctx context.Context, sessionID, page string, userID *primitive.ObjectID) error {
	return s.Track(ctx, models.TelemetryEvent{
		EventName:  models.TelemetryPageView,
		Category:   models.TelemetryCategoryFunnel,
		SessionID:  sessionID,
		UserID:     userID,
		Properties: map[string]interface{}{"page": page},
	})
}

// TrackCheckoutStarted is a convenience method for checkout initiation events.
func (s *Service) TrackCheckoutStarted(ctx context.Context, userID, tenantID primitive.ObjectID, planName string) error {
	return s.Track(ctx, models.TelemetryEvent{
		EventName:  models.TelemetryCheckoutStarted,
		Category:   models.TelemetryCategoryFunnel,
		UserID:     &userID,
		TenantID:   &tenantID,
		Properties: map[string]interface{}{"planName": planName},
	})
}

// TrackLogin is a convenience method for login events.
func (s *Service) TrackLogin(ctx context.Context, userID primitive.ObjectID) error {
	return s.Track(ctx, models.TelemetryEvent{
		EventName: models.TelemetryUserLogin,
		Category:  models.TelemetryCategoryEngagement,
		UserID:    &userID,
	})
}

// --- Query types ---

type FunnelData struct {
	UniqueVisitors   int64        `json:"uniqueVisitors"`
	Registrations    int64        `json:"registrations"`
	PlanPageViews    int64        `json:"planPageViews"`
	CheckoutsStarted int64       `json:"checkoutsStarted"`
	PaidConversions  int64        `json:"paidConversions"`
	Upgrades         int64        `json:"upgrades"`
	Steps            []FunnelStep `json:"steps"`
}

type FunnelStep struct {
	Name       string  `json:"name"`
	Count      int64   `json:"count"`
	Conversion float64 `json:"conversion"`
}

type CohortRow struct {
	CohortLabel string    `json:"cohortLabel"`
	CohortSize  int64     `json:"cohortSize"`
	Retention   []float64 `json:"retention"`
}

type EngagementData struct {
	DAU         []DailyPoint `json:"dau"`
	WAU         []DailyPoint `json:"wau"`
	MAU         []DailyPoint `json:"mau"`
	AvgSessions float64      `json:"avgSessions"`
	TopFeatures []FeatureUse `json:"topFeatures"`
	CreditTrend []DailyPoint `json:"creditTrend"`
}

type DailyPoint struct {
	Date  string `json:"date"`
	Value int64  `json:"value"`
}

type FeatureUse struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type KPIData struct {
	MRR                 int64        `json:"mrr"`
	ARR                 int64        `json:"arr"`
	ARPU                int64        `json:"arpu"`
	LTV                 int64        `json:"ltv"`
	ChurnRate           float64      `json:"churnRate"`
	TrialConversionRate float64      `json:"trialConversionRate"`
	TimeToFirstPurchase float64      `json:"timeToFirstPurchase"`
	ActiveSubscribers   int64        `json:"activeSubscribers"`
	TotalRegistrations  int64        `json:"totalRegistrations"`
	PlanDistribution    []PlanShare  `json:"planDistribution"`
	MRRTrend            []DailyPoint `json:"mrrTrend"`
	SubscriberTrend     []DailyPoint `json:"subscriberTrend"`
}

type PlanShare struct {
	PlanName    string  `json:"planName"`
	Subscribers int64   `json:"subscribers"`
	Percentage  float64 `json:"percentage"`
	MRR         int64   `json:"mrr"`
}

type CustomEventData struct {
	EventName  string       `json:"eventName"`
	TotalCount int64        `json:"totalCount"`
	Trend      []DailyPoint `json:"trend"`
}

type EventTypeSummary struct {
	EventName string    `json:"eventName"`
	Category  string    `json:"category"`
	Count     int64     `json:"count"`
	LastSeen  time.Time `json:"lastSeen"`
}

// --- Query methods ---

// FunnelMetrics computes the conversion funnel for a date range.
func (s *Service) FunnelMetrics(ctx context.Context, start, end time.Time) (*FunnelData, error) {
	dateFilter := bson.M{"createdAt": bson.M{"$gte": start, "$lte": end}}

	// Unique visitors: distinct sessionIDs with page.view
	visitors, _ := s.countDistinct(ctx, "sessionId", bson.M{
		"eventName": models.TelemetryPageView,
		"createdAt": bson.M{"$gte": start, "$lte": end},
	})

	// Registrations: users created in period
	registrations, _ := s.db.Users().CountDocuments(ctx, bson.M{
		"createdAt": bson.M{"$gte": start, "$lte": end},
	})

	// Plan page views: page.view where properties.page = "/plan"
	planViews, _ := s.countDistinct(ctx, "sessionId", bson.M{
		"eventName":       models.TelemetryPageView,
		"properties.page": "/plan",
		"createdAt":       bson.M{"$gte": start, "$lte": end},
	})

	// Checkouts started
	checkouts, _ := s.db.TelemetryEvents().CountDocuments(ctx, bson.M{
		"eventName": models.TelemetryCheckoutStarted,
		"createdAt": bson.M{"$gte": start, "$lte": end},
	})

	// Paid conversions: subscription transactions in period
	conversions, _ := s.db.FinancialTransactions().CountDocuments(ctx, bson.M{
		"type":      models.TransactionSubscription,
		"createdAt": bson.M{"$gte": start, "$lte": end},
	})

	// Plan upgrades
	upgrades, _ := s.db.TelemetryEvents().CountDocuments(ctx, mergeBson(dateFilter, bson.M{
		"eventName": models.TelemetryPlanChanged,
	}))

	steps := buildFunnelSteps(visitors, registrations, planViews, checkouts, conversions, upgrades)

	return &FunnelData{
		UniqueVisitors:   visitors,
		Registrations:    registrations,
		PlanPageViews:    planViews,
		CheckoutsStarted: checkouts,
		PaidConversions:  conversions,
		Upgrades:         upgrades,
		Steps:            steps,
	}, nil
}

func buildFunnelSteps(visitors, registrations, planViews, checkouts, conversions, upgrades int64) []FunnelStep {
	steps := []FunnelStep{
		{Name: "Unique Visitors", Count: visitors},
		{Name: "Registrations", Count: registrations},
		{Name: "Plan Page Views", Count: planViews},
		{Name: "Checkouts Started", Count: checkouts},
		{Name: "Paid Conversions", Count: conversions},
		{Name: "Upgrades", Count: upgrades},
	}
	for i := range steps {
		if i == 0 {
			steps[i].Conversion = 100
		} else if steps[i-1].Count > 0 {
			steps[i].Conversion = math.Round(float64(steps[i].Count)/float64(steps[i-1].Count)*10000) / 100
		}
	}
	return steps
}

// RetentionCohorts computes cohort retention data.
func (s *Service) RetentionCohorts(ctx context.Context, granularity string, periods int) ([]CohortRow, error) {
	if periods <= 0 {
		periods = 12
	}

	var interval time.Duration
	var labelFormat string
	switch granularity {
	case "monthly":
		interval = 30 * 24 * time.Hour
		labelFormat = "2006-01"
	default:
		granularity = "weekly"
		interval = 7 * 24 * time.Hour
		labelFormat = "2006-W02" // ISO week approximation
	}

	now := time.Now()
	rows := make([]CohortRow, 0, periods)

	for i := periods - 1; i >= 0; i-- {
		cohortStart := now.Add(-time.Duration(i+1) * interval)
		cohortEnd := now.Add(-time.Duration(i) * interval)

		// Count users who registered in this cohort window
		cohortFilter := bson.M{
			"createdAt": bson.M{"$gte": cohortStart, "$lt": cohortEnd},
			"isActive":  true,
		}
		cohortSize, _ := s.db.Users().CountDocuments(ctx, cohortFilter)
		if cohortSize == 0 {
			continue
		}

		// For each subsequent period, check how many had lastLoginAt within that window
		retention := make([]float64, 0)
		retention = append(retention, 100) // Period 0 is always 100%

		for p := 1; p <= i; p++ {
			periodStart := cohortEnd.Add(time.Duration(p-1) * interval)
			periodEnd := cohortEnd.Add(time.Duration(p) * interval)

			activeCount, _ := s.db.Users().CountDocuments(ctx, bson.M{
				"createdAt":   bson.M{"$gte": cohortStart, "$lt": cohortEnd},
				"isActive":    true,
				"lastLoginAt": bson.M{"$gte": periodStart, "$lt": periodEnd},
			})

			pct := math.Round(float64(activeCount)/float64(cohortSize)*10000) / 100
			retention = append(retention, pct)
		}

		rows = append(rows, CohortRow{
			CohortLabel: cohortStart.Format(labelFormat),
			CohortSize:  cohortSize,
			Retention:   retention,
		})
	}

	return rows, nil
}

// EngagementMetrics computes engagement data for paying subscribers.
func (s *Service) EngagementMetrics(ctx context.Context, start, end time.Time) (*EngagementData, error) {
	// Get active tenant IDs (paying subscribers)
	activeTenantIDs, err := s.getActiveTenantIDs(ctx)
	if err != nil {
		return &EngagementData{}, nil
	}

	// Get user IDs who belong to active tenants
	activeUserIDs, err := s.getUserIDsForTenants(ctx, activeTenantIDs)
	if err != nil {
		return &EngagementData{}, nil
	}

	// DAU: for each day in range, count distinct users with login events
	dau := s.dailyActiveUsers(ctx, activeUserIDs, start, end)

	// WAU: rolling 7-day windows
	wau := s.weeklyActiveUsers(ctx, activeUserIDs, start, end)

	// MAU: rolling 30-day windows
	mau := s.monthlyActiveUsers(ctx, activeUserIDs, start, end)

	// Average sessions per user per week
	days := end.Sub(start).Hours() / 24
	weeks := days / 7
	if weeks < 1 {
		weeks = 1
	}
	totalLogins, _ := s.db.TelemetryEvents().CountDocuments(ctx, bson.M{
		"eventName": models.TelemetryUserLogin,
		"userId":    bson.M{"$in": activeUserIDs},
		"createdAt": bson.M{"$gte": start, "$lte": end},
	})
	avgSessions := 0.0
	if len(activeUserIDs) > 0 {
		avgSessions = math.Round(float64(totalLogins)/float64(len(activeUserIDs))/weeks*100) / 100
	}

	// Top features: custom events by count
	topFeatures := s.topCustomEvents(ctx, start, end, 10)

	// Credit consumption trend
	creditTrend := s.creditConsumptionTrend(ctx, start, end)

	return &EngagementData{
		DAU:         dau,
		WAU:         wau,
		MAU:         mau,
		AvgSessions: avgSessions,
		TopFeatures: topFeatures,
		CreditTrend: creditTrend,
	}, nil
}

// KPIs computes high-level product management KPIs.
func (s *Service) KPIs(ctx context.Context) (*KPIData, error) {
	now := time.Now()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	prevMonthStart := monthStart.AddDate(0, -1, 0)

	// Active subscribers
	activeSubscribers, _ := s.db.Tenants().CountDocuments(ctx, bson.M{
		"billingStatus": models.BillingStatusActive,
		"isActive":      true,
	})

	// Total registrations
	totalRegistrations, _ := s.db.Users().CountDocuments(ctx, bson.M{})

	// MRR: aggregate from active tenants with plans
	mrr := s.calculateMRR(ctx)
	arr := mrr * 12

	// ARPU
	arpu := int64(0)
	if activeSubscribers > 0 {
		arpu = mrr / activeSubscribers
	}

	// Churn: tenants canceled this month / active at start of month
	canceledThisMonth, _ := s.db.Tenants().CountDocuments(ctx, bson.M{
		"canceledAt": bson.M{"$gte": monthStart},
	})
	activeAtMonthStart, _ := s.db.Tenants().CountDocuments(ctx, bson.M{
		"billingStatus": bson.M{"$in": []string{string(models.BillingStatusActive), string(models.BillingStatusCanceled)}},
		"createdAt":     bson.M{"$lt": monthStart},
	})
	churnRate := 0.0
	if activeAtMonthStart > 0 {
		churnRate = math.Round(float64(canceledThisMonth)/float64(activeAtMonthStart)*10000) / 100
	}

	// Trial conversion
	totalTrials, _ := s.db.Tenants().CountDocuments(ctx, bson.M{
		"trialUsedAt": bson.M{"$ne": nil},
	})
	convertedTrials, _ := s.db.Tenants().CountDocuments(ctx, bson.M{
		"trialUsedAt":  bson.M{"$ne": nil},
		"billingStatus": models.BillingStatusActive,
	})
	trialConversion := 0.0
	if totalTrials > 0 {
		trialConversion = math.Round(float64(convertedTrials)/float64(totalTrials)*10000) / 100
	}

	// Time to first purchase (median)
	ttfp := s.medianTimeToFirstPurchase(ctx)

	// LTV: ARPU / monthly churn rate (simplified)
	ltv := int64(0)
	if churnRate > 0 {
		ltv = int64(float64(arpu) / (churnRate / 100))
	}

	// Plan distribution
	planDist := s.planDistribution(ctx)

	// MRR trend (last 30 days) — use daily_metrics if available
	mrrTrend := s.mrrTrend(ctx, now.AddDate(0, 0, -30), now)

	// Subscriber trend (last 30 days)
	subTrend := s.subscriberTrend(ctx, now.AddDate(0, 0, -30), now)

	_ = prevMonthStart // might use for growth rate later

	return &KPIData{
		MRR:                 mrr,
		ARR:                 arr,
		ARPU:                arpu,
		LTV:                 ltv,
		ChurnRate:           churnRate,
		TrialConversionRate: trialConversion,
		TimeToFirstPurchase: ttfp,
		ActiveSubscribers:   activeSubscribers,
		TotalRegistrations:  totalRegistrations,
		PlanDistribution:    planDist,
		MRRTrend:            mrrTrend,
		SubscriberTrend:     subTrend,
	}, nil
}

// CustomEventSummary returns trend data for a specific event name.
func (s *Service) CustomEventSummary(ctx context.Context, start, end time.Time, eventName string) (*CustomEventData, error) {
	filter := bson.M{
		"createdAt": bson.M{"$gte": start, "$lte": end},
	}
	if eventName != "" {
		filter["eventName"] = eventName
	}

	totalCount, _ := s.db.TelemetryEvents().CountDocuments(ctx, filter)

	// Daily trend
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$createdAt"}},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}

	cursor, err := s.db.TelemetryEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return &CustomEventData{EventName: eventName, TotalCount: totalCount}, nil
	}
	defer cursor.Close(ctx)

	var trend []DailyPoint
	for cursor.Next(ctx) {
		var result struct {
			Date  string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if cursor.Decode(&result) == nil {
			trend = append(trend, DailyPoint{Date: result.Date, Value: result.Count})
		}
	}

	return &CustomEventData{
		EventName:  eventName,
		TotalCount: totalCount,
		Trend:      trend,
	}, nil
}

// ListEventTypes returns all distinct event types with counts.
func (s *Service) ListEventTypes(ctx context.Context) ([]EventTypeSummary, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":      bson.M{"eventName": "$eventName", "category": "$category"},
			"count":    bson.M{"$sum": 1},
			"lastSeen": bson.M{"$max": "$createdAt"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}

	cursor, err := s.db.TelemetryEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []EventTypeSummary
	for cursor.Next(ctx) {
		var result struct {
			ID struct {
				EventName string `bson:"eventName"`
				Category  string `bson:"category"`
			} `bson:"_id"`
			Count    int64     `bson:"count"`
			LastSeen time.Time `bson:"lastSeen"`
		}
		if cursor.Decode(&result) == nil {
			results = append(results, EventTypeSummary{
				EventName: result.ID.EventName,
				Category:  result.ID.Category,
				Count:     result.Count,
				LastSeen:  result.LastSeen,
			})
		}
	}
	if results == nil {
		results = []EventTypeSummary{}
	}
	return results, nil
}

// --- Internal helpers ---

func (s *Service) countDistinct(ctx context.Context, field string, filter bson.M) (int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$group", Value: bson.M{"_id": "$" + field}}},
		{{Key: "$count", Value: "total"}},
	}
	cursor, err := s.db.TelemetryEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		var result struct {
			Total int64 `bson:"total"`
		}
		if cursor.Decode(&result) == nil {
			return result.Total, nil
		}
	}
	return 0, nil
}

func (s *Service) getActiveTenantIDs(ctx context.Context) ([]primitive.ObjectID, error) {
	cursor, err := s.db.Tenants().Find(ctx, bson.M{
		"billingStatus": models.BillingStatusActive,
		"isActive":      true,
	}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var ids []primitive.ObjectID
	for cursor.Next(ctx) {
		var t struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if cursor.Decode(&t) == nil {
			ids = append(ids, t.ID)
		}
	}
	return ids, nil
}

func (s *Service) getUserIDsForTenants(ctx context.Context, tenantIDs []primitive.ObjectID) ([]primitive.ObjectID, error) {
	if len(tenantIDs) == 0 {
		return nil, nil
	}
	cursor, err := s.db.TenantMemberships().Find(ctx, bson.M{
		"tenantId": bson.M{"$in": tenantIDs},
	}, options.Find().SetProjection(bson.M{"userId": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	seen := make(map[primitive.ObjectID]bool)
	var ids []primitive.ObjectID
	for cursor.Next(ctx) {
		var m struct {
			UserID primitive.ObjectID `bson:"userId"`
		}
		if cursor.Decode(&m) == nil && !seen[m.UserID] {
			seen[m.UserID] = true
			ids = append(ids, m.UserID)
		}
	}
	return ids, nil
}

func (s *Service) dailyActiveUsers(ctx context.Context, userIDs []primitive.ObjectID, start, end time.Time) []DailyPoint {
	if len(userIDs) == 0 {
		return nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"eventName": models.TelemetryUserLogin,
			"userId":    bson.M{"$in": userIDs},
			"createdAt": bson.M{"$gte": start, "$lte": end},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"date": bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$createdAt"}}, "userId": "$userId"},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$_id.date",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	return s.aggregateDailyPoints(ctx, pipeline)
}

func (s *Service) weeklyActiveUsers(ctx context.Context, userIDs []primitive.ObjectID, start, end time.Time) []DailyPoint {
	if len(userIDs) == 0 {
		return nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"eventName": models.TelemetryUserLogin,
			"userId":    bson.M{"$in": userIDs},
			"createdAt": bson.M{"$gte": start, "$lte": end},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"week":   bson.M{"$isoWeek": "$createdAt"},
				"year":   bson.M{"$isoWeekYear": "$createdAt"},
				"userId": "$userId",
			},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"week": "$_id.week", "year": "$_id.year"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id.year", Value: 1}, {Key: "_id.week", Value: 1}}}},
	}
	cursor, err := s.db.TelemetryEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var points []DailyPoint
	for cursor.Next(ctx) {
		var result struct {
			ID struct {
				Week int `bson:"week"`
				Year int `bson:"year"`
			} `bson:"_id"`
			Count int64 `bson:"count"`
		}
		if cursor.Decode(&result) == nil {
			label := time.Date(result.ID.Year, 1, 1, 0, 0, 0, 0, time.UTC).
				AddDate(0, 0, (result.ID.Week-1)*7).Format("2006-01-02")
			points = append(points, DailyPoint{Date: label, Value: result.Count})
		}
	}
	return points
}

func (s *Service) monthlyActiveUsers(ctx context.Context, userIDs []primitive.ObjectID, start, end time.Time) []DailyPoint {
	if len(userIDs) == 0 {
		return nil
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"eventName": models.TelemetryUserLogin,
			"userId":    bson.M{"$in": userIDs},
			"createdAt": bson.M{"$gte": start, "$lte": end},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id": bson.M{
				"month":  bson.M{"$month": "$createdAt"},
				"year":   bson.M{"$year": "$createdAt"},
				"userId": "$userId",
			},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"month": "$_id.month", "year": "$_id.year"},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id.year", Value: 1}, {Key: "_id.month", Value: 1}}}},
	}
	cursor, err := s.db.TelemetryEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var points []DailyPoint
	for cursor.Next(ctx) {
		var result struct {
			ID struct {
				Month int `bson:"month"`
				Year  int `bson:"year"`
			} `bson:"_id"`
			Count int64 `bson:"count"`
		}
		if cursor.Decode(&result) == nil {
			label := time.Date(result.ID.Year, time.Month(result.ID.Month), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")
			points = append(points, DailyPoint{Date: label, Value: result.Count})
		}
	}
	return points
}

func (s *Service) topCustomEvents(ctx context.Context, start, end time.Time, limit int) []FeatureUse {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"category":  models.TelemetryCategoryCustom,
			"createdAt": bson.M{"$gte": start, "$lte": end},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$eventName",
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		{{Key: "$limit", Value: limit}},
	}

	cursor, err := s.db.TelemetryEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var features []FeatureUse
	for cursor.Next(ctx) {
		var result struct {
			Name  string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if cursor.Decode(&result) == nil {
			features = append(features, FeatureUse{Name: result.Name, Count: result.Count})
		}
	}
	return features
}

func (s *Service) creditConsumptionTrend(ctx context.Context, start, end time.Time) []DailyPoint {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"createdAt": bson.M{"$gte": start, "$lte": end},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$createdAt"}},
			"total": bson.M{"$sum": "$quantity"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cursor, err := s.db.UsageEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var points []DailyPoint
	for cursor.Next(ctx) {
		var result struct {
			Date  string `bson:"_id"`
			Total int64  `bson:"total"`
		}
		if cursor.Decode(&result) == nil {
			points = append(points, DailyPoint{Date: result.Date, Value: result.Total})
		}
	}
	return points
}

func (s *Service) calculateMRR(ctx context.Context) int64 {
	// Join active tenants with their plans to compute MRR
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"billingStatus": models.BillingStatusActive,
			"isActive":      true,
			"planId":        bson.M{"$ne": nil},
		}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "plans",
			"localField":   "planId",
			"foreignField": "_id",
			"as":           "plan",
		}}},
		{{Key: "$unwind", Value: "$plan"}},
		{{Key: "$project", Value: bson.M{
			"billingInterval": 1,
			"seatQuantity":    1,
			"plan":            1,
		}}},
	}

	cursor, err := s.db.Tenants().Aggregate(ctx, pipeline)
	if err != nil {
		return 0
	}
	defer cursor.Close(ctx)

	var totalMRR int64
	for cursor.Next(ctx) {
		var result struct {
			BillingInterval string `bson:"billingInterval"`
			SeatQuantity    int    `bson:"seatQuantity"`
			Plan            struct {
				MonthlyPriceCents int64  `bson:"monthlyPriceCents"`
				AnnualDiscountPct int    `bson:"annualDiscountPct"`
				PricingModel      string `bson:"pricingModel"`
				PerSeatPriceCents int64  `bson:"perSeatPriceCents"`
				IncludedSeats     int    `bson:"includedSeats"`
			} `bson:"plan"`
		}
		if cursor.Decode(&result) != nil {
			continue
		}

		var monthly int64
		if result.Plan.PricingModel == "per_seat" {
			baseMRR := result.Plan.MonthlyPriceCents
			extraSeats := result.SeatQuantity - result.Plan.IncludedSeats
			if extraSeats < 0 {
				extraSeats = 0
			}
			monthly = baseMRR + int64(extraSeats)*result.Plan.PerSeatPriceCents
		} else {
			monthly = result.Plan.MonthlyPriceCents
		}

		if result.BillingInterval == "year" && result.Plan.AnnualDiscountPct > 0 {
			annual := float64(monthly*12) * (1 - float64(result.Plan.AnnualDiscountPct)/100)
			monthly = int64(annual / 12)
		}
		totalMRR += monthly
	}
	return totalMRR
}

func (s *Service) medianTimeToFirstPurchase(ctx context.Context) float64 {
	// Get first transaction per tenant
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"type": models.TransactionSubscription}}},
		{{Key: "$sort", Value: bson.D{{Key: "createdAt", Value: 1}}}},
		{{Key: "$group", Value: bson.M{
			"_id":             "$userId",
			"firstPurchaseAt": bson.M{"$first": "$createdAt"},
		}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "users",
			"localField":   "_id",
			"foreignField": "_id",
			"as":           "user",
		}}},
		{{Key: "$unwind", Value: "$user"}},
		{{Key: "$project", Value: bson.M{
			"daysToPurchase": bson.M{
				"$divide": []interface{}{
					bson.M{"$subtract": []interface{}{"$firstPurchaseAt", "$user.createdAt"}},
					86400000, // ms to days
				},
			},
		}}},
	}

	cursor, err := s.db.FinancialTransactions().Aggregate(ctx, pipeline)
	if err != nil {
		return 0
	}
	defer cursor.Close(ctx)

	var days []float64
	for cursor.Next(ctx) {
		var result struct {
			Days float64 `bson:"daysToPurchase"`
		}
		if cursor.Decode(&result) == nil && result.Days >= 0 {
			days = append(days, result.Days)
		}
	}

	if len(days) == 0 {
		return 0
	}
	sort.Float64s(days)
	mid := len(days) / 2
	if len(days)%2 == 0 {
		return math.Round((days[mid-1]+days[mid])/2*10) / 10
	}
	return math.Round(days[mid]*10) / 10
}

func (s *Service) planDistribution(ctx context.Context) []PlanShare {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"billingStatus": models.BillingStatusActive,
			"isActive":      true,
			"planId":        bson.M{"$ne": nil},
		}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "plans",
			"localField":   "planId",
			"foreignField": "_id",
			"as":           "plan",
		}}},
		{{Key: "$unwind", Value: "$plan"}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"planId": "$planId", "planName": "$plan.name"},
			"count": bson.M{"$sum": 1},
			"mrr":   bson.M{"$sum": "$plan.monthlyPriceCents"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
	}

	cursor, err := s.db.Tenants().Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var shares []PlanShare
	var total int64
	for cursor.Next(ctx) {
		var result struct {
			ID struct {
				PlanName string `bson:"planName"`
			} `bson:"_id"`
			Count int64 `bson:"count"`
			MRR   int64 `bson:"mrr"`
		}
		if cursor.Decode(&result) == nil {
			shares = append(shares, PlanShare{
				PlanName:    result.ID.PlanName,
				Subscribers: result.Count,
				MRR:         result.MRR,
			})
			total += result.Count
		}
	}
	for i := range shares {
		if total > 0 {
			shares[i].Percentage = math.Round(float64(shares[i].Subscribers)/float64(total)*10000) / 100
		}
	}
	return shares
}

func (s *Service) mrrTrend(ctx context.Context, start, end time.Time) []DailyPoint {
	// Use daily_metrics ARR/12 as MRR proxy
	cursor, err := s.db.DailyMetrics().Find(ctx, bson.M{
		"date": bson.M{
			"$gte": start.Format("2006-01-02"),
			"$lte": end.Format("2006-01-02"),
		},
	}, options.Find().SetSort(bson.D{{Key: "date", Value: 1}}))
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var points []DailyPoint
	for cursor.Next(ctx) {
		var m models.DailyMetric
		if cursor.Decode(&m) == nil {
			points = append(points, DailyPoint{Date: m.Date, Value: m.ARR / 12})
		}
	}
	return points
}

func (s *Service) subscriberTrend(ctx context.Context, start, end time.Time) []DailyPoint {
	// Count active tenants per day using transactions as proxy
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"type":      models.TransactionSubscription,
			"createdAt": bson.M{"$gte": start, "$lte": end},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":   bson.M{"$dateToString": bson.M{"format": "%Y-%m-%d", "date": "$createdAt"}},
			"count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}

	cursor, err := s.db.FinancialTransactions().Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var points []DailyPoint
	for cursor.Next(ctx) {
		var result struct {
			Date  string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if cursor.Decode(&result) == nil {
			points = append(points, DailyPoint{Date: result.Date, Value: result.Count})
		}
	}
	return points
}

func (s *Service) aggregateDailyPoints(ctx context.Context, pipeline mongo.Pipeline) []DailyPoint {
	cursor, err := s.db.TelemetryEvents().Aggregate(ctx, pipeline)
	if err != nil {
		return nil
	}
	defer cursor.Close(ctx)

	var points []DailyPoint
	for cursor.Next(ctx) {
		var result struct {
			Date  string `bson:"_id"`
			Count int64  `bson:"count"`
		}
		if cursor.Decode(&result) == nil {
			points = append(points, DailyPoint{Date: result.Date, Value: result.Count})
		}
	}
	return points
}

func mergeBson(a, b bson.M) bson.M {
	result := bson.M{}
	for k, v := range a {
		result[k] = v
	}
	for k, v := range b {
		result[k] = v
	}
	return result
}
