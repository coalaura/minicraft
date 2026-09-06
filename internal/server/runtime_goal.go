package server

import "sort"

type runtimeGoalFlag uint8

const (
	runtimeGoalMove runtimeGoalFlag = 1 << iota
	runtimeGoalLook
	runtimeGoalJump
	runtimeGoalTarget
)

type runtimeGoal interface {
	CanUse(*Runtime) bool
	CanContinue(*Runtime) bool
	Start(*Runtime)
	Stop(*Runtime)
	Tick(*Runtime)
}

type runtimeGoalEntry struct {
	Goal     runtimeGoal
	Priority int
	Flags    runtimeGoalFlag
	Running  bool
	Order    int
}

type runtimeGoalSelector struct {
	Entries []runtimeGoalEntry
	order   int
}

func (selector *runtimeGoalSelector) Add(priority int, flags runtimeGoalFlag, goal runtimeGoal) {
	entry := runtimeGoalEntry{
		Goal:     goal,
		Priority: priority,
		Flags:    flags,
		Order:    selector.order,
	}

	selector.order++
	selector.Entries = append(selector.Entries, entry)

	sort.SliceStable(selector.Entries, func(left, right int) bool {
		return selector.Entries[left].Priority < selector.Entries[right].Priority
	})
}

func (selector *runtimeGoalSelector) Tick(runtime *Runtime) {
	for index := range selector.Entries {
		entry := &selector.Entries[index]
		if entry.Running && !entry.Goal.CanContinue(runtime) {
			entry.Goal.Stop(runtime)
			entry.Running = false
		}
	}

	for index := range selector.Entries {
		entry := &selector.Entries[index]
		if entry.Running || !entry.Goal.CanUse(runtime) || selector.blocked(index) {
			continue
		}

		selector.stopConflictingLowerPriority(runtime, index)

		entry.Goal.Start(runtime)

		entry.Running = true
	}

	for index := range selector.Entries {
		entry := &selector.Entries[index]
		if entry.Running {
			entry.Goal.Tick(runtime)
		}
	}
}

func (selector *runtimeGoalSelector) blocked(candidateIndex int) bool {
	candidate := selector.Entries[candidateIndex]

	for index := range selector.Entries {
		entry := selector.Entries[index]
		if !entry.Running || entry.Flags&candidate.Flags == 0 {
			continue
		}

		if entry.Priority <= candidate.Priority {
			return true
		}
	}

	return false
}

func (selector *runtimeGoalSelector) stopConflictingLowerPriority(runtime *Runtime, candidateIndex int) {
	candidate := selector.Entries[candidateIndex]

	for index := range selector.Entries {
		entry := &selector.Entries[index]
		if !entry.Running || entry.Priority <= candidate.Priority || entry.Flags&candidate.Flags == 0 {
			continue
		}

		entry.Goal.Stop(runtime)

		entry.Running = false
	}
}
