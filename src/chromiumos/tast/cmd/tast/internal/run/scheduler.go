// Copyright 2020 The Chromium OS Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// This file is for scheduling tests based on precondition tree.
package run

import (
	"fmt"
	"sort"
)

type scheduler struct {
	root *Node

	// intermediate data structure
}

type orderType int

const (
	// Precondition shuold be closed even if prepare failed, and test doesn't run.
	CLOSE_PRECONDITION orderType = iota
	CLEAN_PRECONDITION
	RUN_TEST
)

type runRequest struct {
	// testName is the test to run, or an empty string if only preconditions should run.
	testName string
}

type request struct {
	name   string
	order  orderType
	bundle bundleType
}

type reqRunner func(*request) error

func newScheduler() *scheduler {
	return &scheduler{}
}

func (s *scheduler) addRemotePrecondition(name, parent string) {

}

func (s *scheduler) addLocalPrecondition(name, parent string) {

}

func (s *scheduler) addTest(name, pre string) {

}

func (s *scheduler) run() {

}

type Node struct { // precondition node
	Name  string     // precondition name
	Ty    bundleType // local or remote
	Tests []string   // associated test names
	Cs    []*Node    // children
}

type test struct {
	name         string
	precondition *Node
}

type plan struct {
	// remotePre
}

// returns executed preconditions.
// run precondition Prepare()s, and the test.
func (t *test) execute() ([]*Node, error) {
	panic("todo")
}

func (n *Node) generatePlan() plan {
	panic("todo")
	// for _, c := range n.cs {

	// }
}

func assembleTree(localPres, remotePres, tests map[string]string) (*Node, error) {
	type pt struct {
		ty     bundleType
		parent string
	}
	pres := make(map[string]pt)
	for k, v := range localPres {
		pres[k] = pt{local, v}
	}
	for k, v := range remotePres {
		if _, ok := pres[k]; ok {
			return nil, fmt.Errorf("duplicated preconditions %s", k)
		}
		pres[k] = pt{remote, v}
	}
	if _, ok := pres[""]; ok {
		return nil, fmt.Errorf("empty precondition name is not allowed")
	}
	pres[""] = pt{remote, ""}

	for k, v := range pres {
		p, ok := pres[v.parent]
		if !ok {
			return nil, fmt.Errorf("non-existent parent %q", v.parent)
		}
		if v.ty == remote && p.ty == local {
			return nil, fmt.Errorf("remote precondition %q cannot depend on local precondition %q", k, v.parent)
		}
	}

	status := make(map[string]*int) // 0: unvisited, 1: visited, 2: visiting
	for k := range pres {
		status[k] = new(int)
	}

	var hasCycle func(string) bool
	hasCycle = func(name string) bool {
		p := status[name]
		if *p == 2 {
			return true // found cycle
		}
		if *p == 1 {
			return false
		}
		if name == "" {
			*p = 1
			return false
		}
		*p = 2
		res := hasCycle(pres[name].parent)
		*p = 1
		return res
	}

	for k := range pres {
		if hasCycle(k) {
			return nil, fmt.Errorf("precondition %q is in a cycle", k)
		}
	}

	nodes := make(map[string]*Node)
	for k, v := range pres {
		nodes[k] = &Node{
			Name: k,
			Ty:   v.ty,
		}
	}

	for k, v := range tests {
		if _, ok := pres[v]; !ok {
			return nil, fmt.Errorf("test %v declares non-existent precondition %q", k, v)
		}
		nodes[v].Tests = append(nodes[v].Tests, k)
	}

	for k, v := range pres {
		if k == "" {
			continue
		}
		nodes[v.parent].Cs = append(nodes[v.parent].Cs, nodes[k])
	}

	for _, n := range nodes {
		sort.Strings(n.Tests)
		sort.Slice(n.Cs, func(i, j int) bool { return n.Cs[i].Name < n.Cs[j].Name })
	}

	return nodes[""], nil
}
