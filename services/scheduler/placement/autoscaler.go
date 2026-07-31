package placement

import (
	"fmt"
	"log/slog"
	"sync"
	"time"

	pb "github.com/onlyarnav/nimbusdb/services/metadata-service/proto"
)

type AutoScaleAction string

const (
	ActionScaleOut AutoScaleAction = "scale_out"
	ActionScaleIn  AutoScaleAction = "scale_in"
	ActionNone     AutoScaleAction = "none"
)

type AutoScaleDecision struct {
	Action        AutoScaleAction
	TargetNodeID  string
	Reason        string
	ClusterCPUAvg float32
	ClusterMemAvg float32
}

// AutoScaler evaluates cluster resource utilization and recommends scale-out or scale-in actions.
type AutoScaler struct {
	mu                sync.Mutex
	scaleOutCPUThresh float32 // e.g. 75.0%
	scaleInCPUThresh  float32 // e.g. 20.0%
	cooldownDuration  time.Duration
	lastScaledAt      time.Time
}

// NewAutoScaler initializes an AutoScaler with specified thresholds and cooldown window.
func NewAutoScaler(scaleOutThresh, scaleInThresh float32, cooldown time.Duration) *AutoScaler {
	return &AutoScaler{
		scaleOutCPUThresh: scaleOutThresh,
		scaleInCPUThresh:  scaleInThresh,
		cooldownDuration:  cooldown,
	}
}

// Evaluate analyzes worker node load stats and returns scaling decision.
func (a *AutoScaler) Evaluate(nodes []*pb.NodeInfo) AutoScaleDecision {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.lastScaledAt.IsZero() && time.Since(a.lastScaledAt) < a.cooldownDuration {
		return AutoScaleDecision{
			Action: ActionNone,
			Reason: "cooldown period active",
		}
	}

	var activeNodes []*pb.NodeInfo
	var sumCPU, sumMem float32

	for _, n := range nodes {
		if n.Status == "dead" || n.Status == "draining" || n.Status == "drained" {
			continue
		}
		activeNodes = append(activeNodes, n)
		sumCPU += n.CpuPct
		sumMem += n.MemoryPct
	}

	if len(activeNodes) == 0 {
		return AutoScaleDecision{Action: ActionNone, Reason: "no active nodes"}
	}

	avgCPU := sumCPU / float32(len(activeNodes))
	avgMem := sumMem / float32(len(activeNodes))

	// 1. Scale-out trigger (high load)
	if avgCPU >= a.scaleOutCPUThresh || avgMem >= a.scaleOutCPUThresh {
		a.lastScaledAt = time.Now()
		slog.Warn("auto-scaler decision: SCALE_OUT", "avg_cpu", avgCPU, "avg_mem", avgMem)
		return AutoScaleDecision{
			Action:        ActionScaleOut,
			Reason:        fmt.Sprintf("average CPU (%.1f%%) or Memory (%.1f%%) exceeded threshold (%.1f%%)", avgCPU, avgMem, a.scaleOutCPUThresh),
			ClusterCPUAvg: avgCPU,
			ClusterMemAvg: avgMem,
		}
	}

	// 2. Scale-in trigger (low load, underutilized target node identified for drain)
	if len(activeNodes) > 1 && avgCPU <= a.scaleInCPUThresh && avgMem <= a.scaleInCPUThresh {
		target := activeNodes[0]
		for _, n := range activeNodes[1:] {
			if n.CpuPct < target.CpuPct {
				target = n
			}
		}

		a.lastScaledAt = time.Now()
		slog.Info("auto-scaler decision: SCALE_IN via drain", "target_node", target.Id, "avg_cpu", avgCPU)
		return AutoScaleDecision{
			Action:        ActionScaleIn,
			TargetNodeID:  target.Id,
			Reason:        fmt.Sprintf("average CPU (%.1f%%) below scale-in threshold (%.1f%%)", avgCPU, a.scaleInCPUThresh),
			ClusterCPUAvg: avgCPU,
			ClusterMemAvg: avgMem,
		}
	}

	return AutoScaleDecision{
		Action:        ActionNone,
		Reason:        "cluster utilization within normal bounds",
		ClusterCPUAvg: avgCPU,
		ClusterMemAvg: avgMem,
	}
}
