package scheduler

import (
	"fmt"
	"log"
	"math"

	"github.com/hashicorp/nomad/nomad/mock"
	"github.com/hashicorp/nomad/nomad/structs"
)

const (
	// maxScheduleAttempts is used to limit the number of times
	// we will attempt to schedule if we continue to hit conflicts.
	maxScheduleAttempts = 5
)

// ServiceScheduler is used for 'service' type jobs. This scheduler is
// designed for long-lived services, and as such spends more time attemping
// to make a high quality placement. This is the primary scheduler for
// most workloads.
type ServiceScheduler struct {
	logger  *log.Logger
	state   State
	planner Planner
}

// NewServiceScheduler is a factory function to instantiate a new service scheduler
func NewServiceScheduler(logger *log.Logger, state State, planner Planner) Scheduler {
	s := &ServiceScheduler{
		logger:  logger,
		state:   state,
		planner: planner,
	}
	return s
}

// Process is used to handle a single evaluation
func (s *ServiceScheduler) Process(eval *structs.Evaluation) error {
	// Use the evaluation trigger reason to determine what we need to do
	switch eval.TriggeredBy {
	case structs.EvalTriggerJobRegister:
		return s.handleJobRegister(eval)
	case structs.EvalTriggerJobDeregister:
		return s.evictJobAllocs(eval)
	case structs.EvalTriggerNodeUpdate:
		return s.handleNodeUpdate(eval)
	default:
		return fmt.Errorf("service scheduler cannot handle '%s' evaluation reason",
			eval.TriggeredBy)
	}
}

// handleJobRegister is used to handle a job being registered or updated
func (s *ServiceScheduler) handleJobRegister(eval *structs.Evaluation) error {
	attempts := 0
START:
	// Check the attempt count
	if attempts == maxScheduleAttempts {
		return fmt.Errorf("maximum schedule attempts reached (%d)", attempts)
	}
	attempts += 1

	// Lookup the Job by ID
	job, err := s.state.GetJobByID(eval.JobID)
	if err != nil {
		return fmt.Errorf("failed to get job '%s': %v",
			eval.JobID, err)
	}

	// If the job is missing, maybe a concurrent deregister
	if job == nil {
		s.logger.Printf("[DEBUG] sched: skipping eval %s, job %s not found",
			eval.ID, eval.JobID)
		return nil
	}

	// Materialize all the task groups
	groups := materializeTaskGroups(job)

	// If there is nothing required for this job, treat like a deregister
	if len(groups) == 0 {
		return s.evictJobAllocs(eval)
	}

	// Lookup the allocations by JobID
	allocs, err := s.state.AllocsByJob(eval.JobID)
	if err != nil {
		return fmt.Errorf("failed to get allocs for job '%s': %v",
			eval.JobID, err)
	}

	// TODO: Lookup the nodes the Allocs are on, potentially evict

	// Index the existing allocations
	indexed := indexAllocs(allocs)

	// Diff the required and existing allocations
	place, update, evict, ignore := diffAllocs(job, groups, indexed)
	s.logger.Printf("[DEBUG] sched: eval %s job %s needs %d placements, %d updates, %d evictions, %d ignored allocs",
		eval.ID, eval.JobID, len(place), len(update), len(evict), len(ignore))

	// Fast-pass if nothing to do
	if len(place) == 0 && len(update) == 0 && len(evict) == 0 {
		return nil
	}

	// Start a plan for this evaluation
	plan := eval.MakePlan(job)

	// Add all the evicts
	addEvictsToPlan(plan, evict, indexed)

	// For simplicity, we treat all updates as an evict + place.
	// XXX: This should be done with rolling in-place updates instead.
	addEvictsToPlan(plan, update, indexed)
	place = append(place, update...)

	// Get the iteration stack
	stack, err := s.iterStack(job, plan)
	if err != nil {
		return fmt.Errorf("failed to create iter stack: %v", err)
	}

	// Attempt to place all the allocations
	if err := s.planAllocations(stack, job, plan, place, groups); err != nil {
		return fmt.Errorf("failed to plan allocations: %v", err)
	}

	// Submit the plan
	planResult, newState, err := s.planner.SubmitPlan(plan)
	if err != nil {
		return err
	}

	// If we got a state refresh, try again to ensure we
	// are not missing any allocations
	if newState != nil {
		s.state = newState
		stack.Context.SetState(newState)
		goto START
	}

	// Try again if the plan was not fully committed
	fullCommit, expected, actual := planResult.FullCommit(plan)
	if !fullCommit {
		s.logger.Printf("[DEBUG] sched: eval %s job %s attempted %d placements, %d placed",
			eval.ID, eval.JobID, expected, actual)
		goto START
	}
	return nil
}

// IteratorStack is used to hold pointers to each of the
// iterators which are chained together to do selection.
// Half of the stack is used for feasibility checking, while
// the second half of the stack is used for ranking and selection.
type IteratorStack struct {
	Context             *EvalContext
	BaseNodes           []*structs.Node
	Source              *StaticIterator
	JobConstraint       *ConstraintIterator
	TaskGroupDrivers    *DriverIterator
	TaskGroupConstraint *ConstraintIterator
	RankSource          *FeasibleRankIterator
	BinPack             *BinPackIterator
	Limit               *LimitIterator
	MaxScore            *MaxScoreIterator
}

// iterStack is used to get a set of base nodes and to
// initialize the entire stack of iterators.
func (s *ServiceScheduler) iterStack(job *structs.Job,
	plan *structs.Plan) (*IteratorStack, error) {
	// Create a new stack
	stack := new(IteratorStack)

	// Create an evaluation context
	stack.Context = NewEvalContext(s.state, plan, s.logger)

	// Get the base nodes
	nodes, err := s.baseNodes(job)
	if err != nil {
		return nil, err
	}
	stack.BaseNodes = nodes

	// Create the source iterator. We randomize the order we visit nodes
	// to reduce collisions between schedulers and to do a basic load
	// balancing across eligible nodes.
	stack.Source = NewRandomIterator(stack.Context, stack.BaseNodes)

	// Attach the job constraints.
	stack.JobConstraint = NewConstraintIterator(stack.Context, stack.Source, job.Constraints)

	// Create the task group filters, this must be filled in later
	stack.TaskGroupDrivers = NewDriverIterator(stack.Context, stack.JobConstraint, nil)
	stack.TaskGroupConstraint = NewConstraintIterator(stack.Context, stack.TaskGroupDrivers, nil)

	// Upgrade from feasible to rank iterator
	stack.RankSource = NewFeasibleRankIterator(stack.Context, stack.TaskGroupConstraint)

	// Apply the bin packing, this depends on the resources needed by
	// a particular task group.
	// TODO: Support eviction in the future
	stack.BinPack = NewBinPackIterator(stack.Context, stack.RankSource, nil, false, job.Priority)

	// Apply a limit function. This is to avoid scanning *every* possible node.
	// Instead we need to visit "enough". Using a log of the total number of
	// nodes is a good restriction, with at least 2 as the floor
	limit := 2
	if n := len(nodes); n > 0 {
		logLimit := int(math.Ceil(math.Log2(float64(n))))
		if logLimit > limit {
			limit = logLimit
		}
	}
	stack.Limit = NewLimitIterator(stack.Context, stack.BinPack, limit)

	// Select the node with the maximum score for placement
	stack.MaxScore = NewMaxScoreIterator(stack.Context, stack.Limit)

	return stack, nil
}

// baseNodes returns all the ready nodes in a datacenter that this
// job has specified is usable.
func (s *ServiceScheduler) baseNodes(job *structs.Job) ([]*structs.Node, error) {
	var out []*structs.Node
	for _, dc := range job.Datacenters {
		iter, err := s.state.NodesByDatacenterStatus(dc, structs.NodeStatusReady)
		if err != nil {
			return nil, err
		}
		for {
			raw := iter.Next()
			if raw == nil {
				break
			}
			out = append(out, raw.(*structs.Node))
		}
	}
	return out, nil
}

func (s *ServiceScheduler) planAllocations(stack *IteratorStack, job *structs.Job, plan *structs.Plan,
	place []allocNameID, groups map[string]*structs.TaskGroup) error {

	// Attempt to place each missing allocation
	for _, missing := range place {
		taskGroup := groups[missing.Name]

		// Collect the constraints, drivers and resources required by each
		// sub-task to aggregate the TaskGroup totals
		constr := make([]*structs.Constraint, 0, len(taskGroup.Constraints))
		drivers := make(map[string]struct{})
		size := new(structs.Resources)
		constr = append(constr, taskGroup.Constraints...)
		for _, task := range taskGroup.Tasks {
			drivers[task.Driver] = struct{}{}
			constr = append(constr, task.Constraints...)
			size.Add(task.Resources)
		}

		// Update the parameters of iterators
		stack.MaxScore.Reset()
		stack.TaskGroupDrivers.SetDrivers(drivers)
		stack.TaskGroupConstraint.SetConstraints(constr)
		stack.BinPack.SetResources(size)

		// Select the best fit
		option := stack.MaxScore.Next()
		if option == nil {
			s.logger.Printf("[DEBUG] sched: failed to place alloc %s for job %s",
				missing, job.ID)
			continue
		}

		// Create an allocation for this
		alloc := &structs.Allocation{
			ID:        mock.GenerateUUID(),
			Name:      missing.Name,
			NodeID:    option.Node.ID,
			JobID:     job.ID,
			Job:       job,
			Resources: size,
			Metrics:   nil,
			Status:    structs.AllocStatusPending,
		}
		plan.AppendAlloc(alloc)
	}
	return nil
}

// handleNodeUpdate is used to handle an update to a node status where
// there is an existing allocation for this job
func (s *ServiceScheduler) handleNodeUpdate(eval *structs.Evaluation) error {
	// TODO
	return nil
}

// evictJobAllocs is used to evict all job allocations
func (s *ServiceScheduler) evictJobAllocs(eval *structs.Evaluation) error {
START:
	// Lookup the allocations by JobID
	allocs, err := s.state.AllocsByJob(eval.JobID)
	if err != nil {
		return fmt.Errorf("failed to get allocs for job '%s': %v",
			eval.JobID, err)
	}

	// Nothing to do if there is no evictsion
	s.logger.Printf("[DEBUG] sched: eval %s job %s needs %d evictions",
		eval.ID, eval.JobID, len(allocs))
	if len(allocs) == 0 {
		return nil
	}

	// Create a plan to evict these
	plan := &structs.Plan{
		EvalID:    eval.ID,
		Priority:  eval.Priority,
		NodeEvict: make(map[string][]string),
	}

	// Add each alloc to be evicted
	for _, alloc := range allocs {
		plan.AppendEvict(alloc)
	}

	// Submit the plan
	_, newState, err := s.planner.SubmitPlan(plan)
	if err != nil {
		return err
	}

	// If we got a state refresh, try again to ensure we
	// are not missing any allocations
	if newState != nil {
		s.state = newState
		goto START
	}
	return nil
}
