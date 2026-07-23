package region

import (
	"sort"
	"strings"
)

// Known regions
const (
	RegionIndia  = "india"
	RegionUSEast = "us-east"
	RegionUSWest = "us-west"
	RegionEurope = "europe"
	RegionJapan  = "japan"
)

// RegionStatus enum
type RegionStatus string

const (
	StatusHealthy  RegionStatus = "healthy"
	StatusDegraded RegionStatus = "degraded"
	StatusDown     RegionStatus = "down"
)

// SupportedRegions is the canonical list of simulated regions.
var SupportedRegions = []string{
	RegionIndia,
	RegionUSEast,
	RegionUSWest,
	RegionEurope,
	RegionJapan,
}

// LatencyMatrix defines the synthetic inter-region latency matrix (in milliseconds).
var LatencyMatrix = map[string]map[string]int{
	RegionIndia: {
		RegionIndia:  0,
		RegionUSEast: 180,
		RegionUSWest: 220,
		RegionEurope: 110,
		RegionJapan:  130,
	},
	RegionUSEast: {
		RegionIndia:  180,
		RegionUSEast: 0,
		RegionUSWest: 60,
		RegionEurope: 90,
		RegionJapan:  160,
	},
	RegionUSWest: {
		RegionIndia:  220,
		RegionUSEast: 60,
		RegionUSWest: 0,
		RegionEurope: 140,
		RegionJapan:  110,
	},
	RegionEurope: {
		RegionIndia:  110,
		RegionUSEast: 90,
		RegionUSWest: 140,
		RegionEurope: 0,
		RegionJapan:  210,
	},
	RegionJapan: {
		RegionIndia:  130,
		RegionUSEast: 160,
		RegionUSWest: 110,
		RegionEurope: 210,
		RegionJapan:  0,
	},
}

// NodeState representation for rollup calculation
type NodeState struct {
	ID     string
	Region string
	Status string // "healthy", "unhealthy", "dead", "unknown"
}

// RegionHealthInfo holds the summary health for a single region
type RegionHealthInfo struct {
	Region         string       `json:"region"`
	Status         RegionStatus `json:"status"`
	TotalNodes     int          `json:"total_nodes"`
	HealthyNodes   int          `json:"healthy_nodes"`
	UnhealthyNodes int          `json:"unhealthy_nodes"`
	DeadNodes      int          `json:"dead_nodes"`
}

// RollupRegionHealth computes aggregate region health for a given region from its node states.
func RollupRegionHealth(regionName string, nodes []NodeState) RegionHealthInfo {
	info := RegionHealthInfo{
		Region: regionName,
		Status: StatusDown,
	}

	for _, n := range nodes {
		if !strings.EqualFold(n.Region, regionName) {
			continue
		}
		info.TotalNodes++
		switch strings.ToLower(n.Status) {
		case "healthy":
			info.HealthyNodes++
		case "unhealthy":
			info.UnhealthyNodes++
		case "dead":
			info.DeadNodes++
		default:
			info.UnhealthyNodes++
		}
	}

	if info.TotalNodes == 0 || info.HealthyNodes == 0 {
		info.Status = StatusDown
	} else if info.UnhealthyNodes > 0 || info.DeadNodes > 0 {
		info.Status = StatusDegraded
	} else {
		info.Status = StatusHealthy
	}

	return info
}

// GetFallbackRegions returns candidate regions ordered by synthetic latency relative to preferredRegion.
func GetFallbackRegions(preferredRegion string) []string {
	pref := strings.ToLower(preferredRegion)
	latencies, exists := LatencyMatrix[pref]
	if !exists {
		// Fallback to india if unknown region hint provided
		pref = RegionIndia
		latencies = LatencyMatrix[pref]
	}

	type regionDistance struct {
		name     string
		distance int
	}

	var candidates []regionDistance
	for r, dist := range latencies {
		candidates = append(candidates, regionDistance{name: r, distance: dist})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].distance < candidates[j].distance
	})

	var result []string
	for _, c := range candidates {
		result = append(result, c.name)
	}
	return result
}
