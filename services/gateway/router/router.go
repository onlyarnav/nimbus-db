package router

import (
	"errors"
	"fmt"
	"strings"

	"github.com/onlyarnav/nimbusdb/services/metadata-service/region"
)

var ErrNoHealthyRegions = errors.New("all simulated regions are currently down")

// RouteResult describes the target region chosen by the gateway router.
type RouteResult struct {
	PreferredRegion  string
	ServedRegion     string
	FallbackRerouted bool
	Reason           string
}

// SelectRegion selects the best target region given a preferred region hint and the current region health state.
func SelectRegion(preferredRegion string, regionHealth map[string]region.RegionStatus) (*RouteResult, error) {
	pref := strings.ToLower(preferredRegion)
	if pref == "" {
		pref = region.RegionIndia
	}

	fallbackOrder := region.GetFallbackRegions(pref)

	for i, regName := range fallbackOrder {
		status, exists := regionHealth[regName]
		if !exists {
			status = region.StatusDown
		}

		if status == region.StatusHealthy || status == region.StatusDegraded {
			rerouted := (i > 0)
			var reason string
			if rerouted {
				reason = fmt.Sprintf("Preferred region %q is DOWN; rerouted to next nearest healthy region %q", pref, regName)
			} else {
				reason = fmt.Sprintf("Routed directly to preferred region %q", pref)
			}
			return &RouteResult{
				PreferredRegion:  pref,
				ServedRegion:     regName,
				FallbackRerouted: rerouted,
				Reason:           reason,
			}, nil
		}
	}

	return nil, ErrNoHealthyRegions
}
