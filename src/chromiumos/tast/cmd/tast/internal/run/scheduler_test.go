package run

import (
	"chromiumos/tast/rpc"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestGenerateTestRequest(t *testing.T) {
	for _, tc := range []struct {
		name string
		tree *Node
		want []*rpc.TestRequest
	}{
		{
			name: "nest",
			/*
				.[x]
				└── a
				    ├── b[y,z]
				    └── c[w]
			*/
			tree: &Node{
				Name:  "",
				Ty:    remote,
				Tests: []string{"x"},
				Cs: []*Node{
					{
						Name: "a",
						Ty:   remote,
						Cs: []*Node{
							{
								Name:  "b",
								Ty:    local,
								Tests: []string{"y", "z"}},
							{
								Name:  "c",
								Ty:    local,
								Tests: []string{"w"},
							},
						},
					},
				},
			},
			want: []*rpc.TestRequest{
				{
					Name: "x",
				},
				{
					Name: "y",
					Precondition: &rpc.Precondition{
						Name:       "b",
						BundleType: rpc.BundleType_LOCAL,
						Parent: &rpc.Precondition{
							Name:       "a",
							BundleType: rpc.BundleType_REMOTE,
						},
					},
				},
				{
					Name: "z",
					Precondition: &rpc.Precondition{
						Name:       "b",
						BundleType: rpc.BundleType_LOCAL,
						Parent: &rpc.Precondition{
							Name:       "a",
							BundleType: rpc.BundleType_REMOTE,
						},
						ShouldClose: true,
					},
				},
				{
					Name: "w",
					Precondition: &rpc.Precondition{
						Name:       "c",
						BundleType: rpc.BundleType_LOCAL,
						Parent: &rpc.Precondition{
							Name:        "a",
							BundleType:  rpc.BundleType_REMOTE,
							ShouldClose: true,
						},
						ShouldClose: true,
					},
				},
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := tc.tree.generateTestRequests()

			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("(-got +want): %v", diff)
			}
		})
	}
}

func TestAssembleTree(t *testing.T) {
	for _, tc := range []struct {
		name    string
		local   map[string]string
		remote  map[string]string
		tests   map[string]string
		wantErr string
		want    *Node
	}{
		{
			name: "empty",
			want: &Node{
				Name: "",
				Ty:   remote,
			},
		},
		{
			name:   "correct tree",
			local:  map[string]string{"a": "b", "b": "r", "c": "r", "d": ""},
			remote: map[string]string{"r": "s", "s": ""},
			tests:  map[string]string{"x": "a", "y": "r", "z": "", "w": ""},
			want: &Node{
				Name:  "",
				Ty:    remote,
				Tests: []string{"w", "z"},
				Cs: []*Node{
					{
						Name: "d",
						Ty:   local,
					},
					{
						Name: "s",
						Ty:   remote,
						Cs: []*Node{
							{
								Name:  "r",
								Ty:    remote,
								Tests: []string{"y"},
								Cs: []*Node{
									{
										Name: "b",
										Ty:   local,
										Cs: []*Node{
											{
												Name:  "a",
												Ty:    local,
												Tests: []string{"x"},
											},
										},
									},
									{
										Name: "c",
										Ty:   local,
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:    "remote to local dependeny",
			local:   map[string]string{"b": ""},
			remote:  map[string]string{"a": "b"},
			wantErr: "cannot depend",
		},
		{
			name:    "same name",
			local:   map[string]string{"a": ""},
			remote:  map[string]string{"a": ""},
			wantErr: "duplicated",
		},
		{
			name:    "cycle in local",
			local:   map[string]string{"a": "b", "b": "a"},
			wantErr: "cycle",
		},
		{
			name:    "cycle in remote",
			remote:  map[string]string{"a": "b", "b": "a"},
			wantErr: "cycle",
		},
		{
			name:    "empty precondition name",
			local:   map[string]string{"": "a"},
			wantErr: "empty",
		},
		{
			name:    "non-existent precondition parent",
			local:   map[string]string{"a": "b"},
			wantErr: "non-existent",
		},
		{
			name:    "non-existent test's precondition",
			tests:   map[string]string{"x": "a"},
			wantErr: "non-existent",
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := assembleTree(tc.local, tc.remote, tc.tests)

			if err != nil {
				if tc.wantErr == "" {
					t.Fatalf("got error: %v", err)
				} else if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error must contain %q: %v", tc.wantErr, err)
				}
			} else if tc.wantErr != "" {
				t.Fatalf("err == nil, want %q", tc.wantErr)
			}
			if diff := cmp.Diff(got, tc.want); diff != "" {
				t.Errorf("(-got +want): %v", diff)
			}
		})
	}
}
