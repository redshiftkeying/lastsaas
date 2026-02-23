package health

import (
	"context"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"lastsaas/internal/apicounter"
	"lastsaas/internal/db"
	"lastsaas/internal/middleware"
	"lastsaas/internal/models"
	"lastsaas/internal/version"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Service manages node registration, metric collection, and queries.
type Service struct {
	db        *db.MongoDB
	metrics   *middleware.MetricsCollector
	getConfig func(string) string
	nodeID    string
	stopCh    chan struct{}

	// Integration health checks
	integrations []integrationEntry
	intMu        sync.RWMutex
	intResults   []models.IntegrationCheck
}

// New creates a health monitoring Service.
func New(database *db.MongoDB, metricsCollector *middleware.MetricsCollector, getConfig func(string) string) *Service {
	nodeID := os.Getenv("FLY_MACHINE_ID")
	if nodeID == "" {
		h, _ := os.Hostname()
		nodeID = h
	}
	return &Service{
		db:        database,
		metrics:   metricsCollector,
		getConfig: getConfig,
		nodeID:    nodeID,
		stopCh:    make(chan struct{}),
	}
}

// Start launches the heartbeat, collector, and integration check background goroutines.
func (s *Service) Start() {
	go s.heartbeatLoop()
	go s.collectorLoop()
	go s.integrationCheckLoop()
	log.Printf("Health monitoring started (node: %s)", s.nodeID)
}

// Stop signals background goroutines to halt.
func (s *Service) Stop() {
	close(s.stopCh)
}

func (s *Service) heartbeatLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("health: heartbeat recovered from panic: %v", r)
		}
	}()

	s.registerNode()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.heartbeat()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Service) registerNode() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	now := time.Now()
	_, err := s.db.SystemNodes().UpdateOne(ctx,
		bson.M{"machineId": s.nodeID},
		bson.M{
			"$set": bson.M{
				"hostname":  hostname(),
				"status":    models.NodeStatusActive,
				"lastSeen":  now,
				"version":   version.Current,
				"goVersion": runtime.Version(),
			},
			"$setOnInsert": bson.M{
				"_id":       primitive.NewObjectID(),
				"machineId": s.nodeID,
				"startedAt": now,
			},
		},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		log.Printf("health: failed to register node: %v", err)
	}
}

func (s *Service) heartbeat() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := s.db.SystemNodes().UpdateOne(ctx,
		bson.M{"machineId": s.nodeID},
		bson.M{"$set": bson.M{"lastSeen": time.Now(), "status": models.NodeStatusActive}},
	)
	if err != nil {
		log.Printf("health: heartbeat failed: %v", err)
	}
}

func (s *Service) collectorLoop() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("health: collector recovered from panic: %v", r)
		}
	}()

	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.collectAndStore()
		case <-s.stopCh:
			return
		}
	}
}

func (s *Service) collectAndStore() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	metric := models.SystemMetric{
		ID:        primitive.NewObjectID(),
		NodeID:    s.nodeID,
		Timestamp: time.Now(),
	}

	// CPU
	if cpuPcts, err := cpu.PercentWithContext(ctx, time.Second, false); err == nil && len(cpuPcts) > 0 {
		metric.CPU = models.CPUMetrics{
			UsagePercent: cpuPcts[0],
			NumCPU:       runtime.NumCPU(),
		}
	} else if err != nil {
		log.Printf("health: cpu collect error: %v", err)
	}

	// Memory
	if vm, err := mem.VirtualMemoryWithContext(ctx); err == nil {
		metric.Memory = models.MemoryMetrics{
			UsedBytes:   vm.Used,
			TotalBytes:  vm.Total,
			UsedPercent: vm.UsedPercent,
		}
	} else {
		log.Printf("health: memory collect error: %v", err)
	}

	// Disk
	if du, err := disk.UsageWithContext(ctx, "/"); err == nil {
		metric.Disk = models.DiskMetrics{
			UsedBytes:   du.Used,
			TotalBytes:  du.Total,
			UsedPercent: du.UsedPercent,
		}
	} else {
		log.Printf("health: disk collect error: %v", err)
	}

	// Network
	if counters, err := net.IOCountersWithContext(ctx, false); err == nil && len(counters) > 0 {
		metric.Network = models.NetworkMetrics{
			BytesSent: counters[0].BytesSent,
			BytesRecv: counters[0].BytesRecv,
		}
	} else if err != nil {
		log.Printf("health: network collect error: %v", err)
	}

	// HTTP from middleware
	snap := s.metrics.Snapshot()
	total := snap.Status2xx + snap.Status3xx + snap.Status4xx + snap.Status5xx
	var err4xx, err5xx float64
	if total > 0 {
		err4xx = float64(snap.Status4xx) / float64(total) * 100
		err5xx = float64(snap.Status5xx) / float64(total) * 100
	}
	metric.HTTP = models.HTTPMetrics{
		RequestCount: snap.RequestCount,
		LatencyP50:   snap.LatencyP50,
		LatencyP95:   snap.LatencyP95,
		LatencyP99:   snap.LatencyP99,
		StatusCodes: map[string]int64{
			"2xx": snap.Status2xx,
			"3xx": snap.Status3xx,
			"4xx": snap.Status4xx,
			"5xx": snap.Status5xx,
		},
		ErrorRate4xx: err4xx,
		ErrorRate5xx: err5xx,
	}

	// MongoDB stats
	metric.Mongo = s.collectMongoMetrics(ctx)

	// Go runtime
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	var gcPause uint64
	if bi, ok := debug.ReadBuildInfo(); ok {
		_ = bi // just checking debug is available
	}
	if memStats.NumGC > 0 {
		gcPause = memStats.PauseNs[(memStats.NumGC+255)%256]
	}
	metric.GoRuntime = models.GoRuntimeMetrics{
		NumGoroutine: runtime.NumGoroutine(),
		HeapAlloc:    memStats.HeapAlloc,
		HeapSys:      memStats.HeapSys,
		GCPauseNs:    gcPause,
		NumGC:        memStats.NumGC,
	}

	// Integration API call counters (snapshot and reset)
	metric.Integrations = models.IntegrationCountMetrics{
		StripeAPICalls: apicounter.StripeAPICalls.Swap(0),
		ResendEmails:   apicounter.ResendEmails.Swap(0),
	}

	if _, err := s.db.SystemMetrics().InsertOne(ctx, metric); err != nil {
		log.Printf("health: failed to store metrics: %v", err)
	}
}

func (s *Service) collectMongoMetrics(ctx context.Context) models.MongoMetrics {
	var result models.MongoMetrics

	// serverStatus
	var serverStatus bson.M
	if err := s.db.Database.RunCommand(ctx, bson.D{{Key: "serverStatus", Value: 1}}).Decode(&serverStatus); err == nil {
		if conns, ok := serverStatus["connections"].(bson.M); ok {
			result.CurrentConnections = toInt32(conns["current"])
			result.AvailableConnections = toInt32(conns["available"])
		}
		if opcounters, ok := serverStatus["opcounters"].(bson.M); ok {
			result.OpCounters = make(map[string]int64)
			for _, key := range []string{"insert", "query", "update", "delete"} {
				if v, exists := opcounters[key]; exists {
					result.OpCounters[key] = toInt64(v)
				}
			}
		}
	} else {
		log.Printf("health: serverStatus error: %v", err)
	}

	// dbStats
	var dbStats bson.M
	if err := s.db.Database.RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}}).Decode(&dbStats); err == nil {
		result.DataSizeBytes = toInt64(dbStats["dataSize"])
		result.IndexSizeBytes = toInt64(dbStats["indexSize"])
		result.Collections = toInt32(dbStats["collections"])
	} else {
		log.Printf("health: dbStats error: %v", err)
	}

	return result
}

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int64:
		return n
	case float64:
		return int64(n)
	default:
		return 0
	}
}

func toInt32(v interface{}) int32 {
	switch n := v.(type) {
	case int32:
		return n
	case int64:
		return int32(n)
	case float64:
		return int32(n)
	default:
		return 0
	}
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}
