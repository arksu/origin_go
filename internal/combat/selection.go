package combat

import (
	"fmt"
	"origin/internal/types"
	"sort"
)

type Candidate struct {
	ID          types.EntityID
	Incarnation types.Handle
	Bounds      AABB
}

type Contact struct {
	Candidate
	Distance float64
}

// Select checks eligibility before geometry and returns stable ID order for application.
// The adapter supplies current-world liveness, incarnation, and receiver HP checks.
func Select(actorID types.EntityID, sector Sector, candidates []Candidate, nearest bool, eligible func(Candidate) bool) ([]Contact, error) {
	if err := sector.Validate(); err != nil {
		return nil, err
	}
	if eligible == nil {
		return nil, fmt.Errorf("receiver eligibility callback is required")
	}
	seen := make(map[types.EntityID]struct{}, len(candidates))
	contacts := make([]Contact, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.ID == 0 || candidate.ID == actorID || !eligible(candidate) {
			continue
		}
		if _, exists := seen[candidate.ID]; exists {
			continue
		}
		seen[candidate.ID] = struct{}{}
		distance, hit, err := ContactDistance(sector, candidate.Bounds)
		if err != nil {
			return nil, fmt.Errorf("receiver %d: %w", candidate.ID, err)
		}
		if hit {
			contacts = append(contacts, Contact{Candidate: candidate, Distance: distance})
		}
	}
	if nearest && len(contacts) > 0 {
		best := contacts[0]
		for _, contact := range contacts[1:] {
			if contact.Distance < best.Distance || (contact.Distance == best.Distance && contact.ID < best.ID) {
				best = contact
			}
		}
		return []Contact{best}, nil
	}
	sort.Slice(contacts, func(left, right int) bool { return contacts[left].ID < contacts[right].ID })
	return contacts, nil
}
