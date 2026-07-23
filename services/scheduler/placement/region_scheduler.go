package placement

import (
	"fmt"
	"strings"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/region"
	pb "github.com/onlyarnav/nimbusdb/services/scheduler/proto"
)

// RegionScheduleResult contains the chosen node and the region that served the request.
type RegionScheduleResult struct {
	Node           *NodeScore
	ServedRegion   string
	FallbackUsed   bool
	FallbackReason string
}

// ScheduleNodeWithRegionFallback attempts to schedule on the preferred region.
// If the preferred region is down or has no available healthy nodes, it tries candidate fallback regions ordered by latency.
func ScheduleNodeWithRegionFallback(nodes []*pb.NodeInfo, preferredRegion string) (*RegionScheduleResult, error) {
	if preferredRegion == "" {
		preferredRegion = region.RegionIndia
	}
	pref := strings.ToLower(preferredRegion)

	// 1. Group nodes by cluster/hostname region prefix or explicit ClusterID
	nodesByRegion := make(map[string][]*pb.NodeInfo)
	for _, n := range nodes {
		reg := inferNodeRegion(n)
		nodesByRegion[reg] = append(nodesByRegion[reg], n)
	}

	// 2. Obtain ordered fallback list starting with preferredRegion
	fallbackOrder := region.GetFallbackRegions(pref)

	var lastErr error
	var fallbackUsed bool
	var fallbackReason string

	for i, regName := range fallbackOrder {
		regNodes := nodesByRegion[regName]
		if len(regNodes) == 0 {
			continue
		}

		score, err := ScheduleNode(regNodes)
		if err == nil {
			if i > 0 {
				fallbackUsed = true
				fallbackReason = fmt.Sprintf("Preferred region %q unavailable; fell back to %q", preferredRegion, regName)
			}
			return &RegionScheduleResult{
				Node:           score,
				ServedRegion:   regName,
				FallbackUsed:   fallbackUsed,
				FallbackReason: fallbackReason,
			}, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return nil, fmt.Errorf("scheduling failed across all regional fallbacks: %w", lastErr)
	}

	return nil, ErrNoNodesAvailable
}

// Helper to infer region from node metadata (e.g. hostname prefix or cluster_id)
func inferNodeRegion(n *pb.NodeInfo) string {
	h := strings.ToLower(n.Hostname)
	c := strings.ToLower(n.ClusterId)

	for _, r := range region.SupportedRegions {
		if strings.Contains(h, r) || strings.Contains(c, r) {
			return r
		}
	}
	return region.RegionIndia
}
