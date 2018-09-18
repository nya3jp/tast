package testing

import "testing"

func TestHookRegistry(t *testing.T) {
	r := newHookRegistry()

	calls := 0
	hook := func(s *State) { calls++ }
	r.addPostHook(hook)

	r.runPostHooks(nil)

	if calls != 1 {
		t.Errorf("Hook should be called exactly once; %d actual", calls)
	}
}
