package run

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

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
			tests:  map[string]string{"x": "a", "y": "r", "z": ""},
			want: &Node{
				Name:  "",
				Ty:    remote,
				Tests: []string{"z"},
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
