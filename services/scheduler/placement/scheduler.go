package placement

import (
	"errors"

	pb "github.com/onlyarnav/nimbusdb/services/scheduler/proto"
)

// ErrNoNodesAvailable is returned when no nodes are available to host database workloads.
var ErrNoNodesAvailable = errors.New("no healthy candidate nodes available in cluster")

// NodeScore contains the node placement details, calculated score, and breakdown metrics.
type NodeScore struct {
	NodeID         string
	Score          float32
	ScoreBreakdown map[string]float32
	Status         string
}

// CalculateScore computes the placement suitability score for a given node.
// Formula: score = (100 - cpu_pct)*0.4 + (100 - memory_pct)*0.3 + (100 - disk_pct)*0.3
func CalculateScore(cpu, mem, disk float32) (float32, map[string]float32) {
	cpuFactor := (100.0 - cpu) * 0.4
	memFactor := (100.0 - mem) * 0.3
	diskFactor := (100.0 - disk) * 0.3
	total := cpuFactor + memFactor + diskFactor

	return total, map[string]float32{
		"cpu_factor":    cpuFactor,
		"memory_factor": memFactor,
		"disk_factor":   diskFactor,
	}
}

// ScheduleNode filters, prioritizes, and selects the best node from the given candidates.
func ScheduleNode(nodes []*pb.NodeInfo) (*NodeScore, error) {
	var nonOverloaded []NodeScore
	var overloaded []NodeScore

	for _, n := range nodes {
		// Filter out dead and draining nodes
		if n.Status == "dead" || n.Status == "draining" {
			continue
		}

		score, breakdown := CalculateScore(n.CpuPct, n.MemoryPct, n.DiskPct)
		ns := NodeScore{
			NodeID:         n.Id,
			Score:          score,
			ScoreBreakdown: breakdown,
			Status:         n.Status,
		}

		if n.Status == "overloaded" {
			overloaded = append(overloaded, ns)
		} else {
			nonOverloaded = append(nonOverloaded, ns)
		}
	}

	// 1. Pick from non-overloaded nodes first (healthy, unknown)
	if len(nonOverloaded) > 0 {
		best := nonOverloaded[0]
		for _, n := range nonOverloaded[1:] {
			if n.Score > best.Score {
				best = n
			}
		}
		return &best, nil
	}

	// 2. Pick from overloaded nodes only if no other nodes exist
	if len(overloaded) > 0 {
		best := overloaded[0]
		for _, n := range overloaded[1:] {
			if n.Score > best.Score {
				best = n
			}
		}
		return &best, nil
	}

	return nil, ErrNoNodesAvailable
}
