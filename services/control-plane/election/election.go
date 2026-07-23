package election

import (
	"errors"
	"sort"
	"strings"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/region"
)

var ErrNoEligibleFollower = errors.New("no healthy follower candidate regions available for leader election")

// RegionPriority maps canonical region priority tie-breaker order.
var RegionPriority = map[string]int{
	region.RegionIndia:  1,
	region.RegionUSEast: 2,
	region.RegionUSWest: 3,
	region.RegionEurope: 4,
	region.RegionJapan:  5,
}

// FollowerCandidate represents a follower database replica in a region.
type FollowerCandidate struct {
	Region     string
	NodeID     string
	AppliedLSN uint64
}

// ElectionResult represents the outcome of leader election.
type ElectionResult struct {
	ElectedLeaderRegion string
	ElectedNodeID       string
	AppliedLSN          uint64
	Reason              string
}

// ElectNewLeaderRegion deterministically selects the best follower region to promote to primary leader.
// Selection criteria:
// 1. Highest applied LSN (minimizes data loss)
// 2. Priority tie-breaker (india > us-east > us-west > europe > japan)
func ElectNewLeaderRegion(candidates []FollowerCandidate, regionHealth map[string]region.RegionStatus) (*ElectionResult, error) {
	var eligible []FollowerCandidate

	for _, c := range candidates {
		regName := strings.ToLower(c.Region)
		status, exists := regionHealth[regName]
		if !exists {
			status = region.StatusDown
		}

		if status == region.StatusHealthy || status == region.StatusDegraded {
			eligible = append(eligible, c)
		}
	}

	if len(eligible) == 0 {
		return nil, ErrNoEligibleFollower
	}

	// Sort eligible candidates by LSN desc, then priority asc
	sort.Slice(eligible, func(i, j int) bool {
		if eligible[i].AppliedLSN != eligible[j].AppliedLSN {
			return eligible[i].AppliedLSN > eligible[j].AppliedLSN
		}
		pI := RegionPriority[strings.ToLower(eligible[i].Region)]
		pJ := RegionPriority[strings.ToLower(eligible[j].Region)]
		if pI == 0 {
			pI = 99
		}
		if pJ == 0 {
			pJ = 99
		}
		return pI < pJ
	})

	best := eligible[0]
	return &ElectionResult{
		ElectedLeaderRegion: strings.ToLower(best.Region),
		ElectedNodeID:       best.NodeID,
		AppliedLSN:          best.AppliedLSN,
		Reason:              "Promoted candidate with highest LSN and region priority tie-breaker",
	}, nil
}
