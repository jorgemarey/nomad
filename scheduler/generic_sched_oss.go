// +build !ent

package scheduler

import "github.com/hashicorp/nomad/nomad/structs"

// selectNextOption calls the stack to get a node for placement
func (s *GenericScheduler) selectNextOption(tg *structs.TaskGroup, selectOptions *SelectOptions) *RankedNode {
	selectOptions.Preempt = s.stack.binPack.evict || selectOptions.Preempt
	return s.stack.Select(tg, selectOptions)
}

// handlePreemptions sets relevant preeemption related fields. In OSS this is a no op.
func (s *GenericScheduler) handlePreemptions(option *RankedNode, alloc *structs.Allocation, missing placementResult) {
	if option.PreemptedAllocs != nil {
		var preemptedAllocIDs []string
		for _, stop := range option.PreemptedAllocs {
			s.plan.AppendPreemptedAlloc(stop, alloc.ID)

			preemptedAllocIDs = append(preemptedAllocIDs, stop.ID)
			if s.eval.AnnotatePlan && s.plan.Annotations != nil {
				s.plan.Annotations.PreemptedAllocs = append(s.plan.Annotations.PreemptedAllocs, stop.Stub())
				if s.plan.Annotations.DesiredTGUpdates != nil {
					desired := s.plan.Annotations.DesiredTGUpdates[missing.TaskGroup().Name]
					desired.Preemptions += 1
				}
			}
		}
		alloc.PreemptedAllocations = preemptedAllocIDs
	}
}
