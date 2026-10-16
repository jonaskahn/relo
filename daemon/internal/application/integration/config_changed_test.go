// The repair's restart rule: what decides that a client process has to read
// what Relo just wrote.
package integration

import "testing"

// TestConfigChangedNamesTheWriteThatMovedTheClient walks the states a repair
// can leave behind. A change of state or of staleness, a file that appeared or
// changed, and a file that came or went each mean the client process is still
// reading the previous write. A repair that changed nothing does not.
func TestConfigChangedNamesTheWriteThatMovedTheClient(t *testing.T) {
	settled := FileView{Kind: KindCodexConfig, Present: true}
	tests := []struct {
		name          string
		before, after View
		want          bool
	}{
		{
			name:   "the state moved off on",
			before: View{State: StateOn, Files: []FileView{settled}},
			after:  View{State: StateDrifted, Files: []FileView{settled}},
			want:   true,
		},
		{
			name:   "the models went stale",
			before: View{State: StateOn, Files: []FileView{settled}},
			after:  View{State: StateOn, ModelsStale: true, Files: []FileView{settled}},
			want:   true,
		},
		{
			name:   "the repair wrote a file this setup did not have",
			before: View{State: StateOn, Files: []FileView{settled}},
			after:  View{State: StateOn, Files: []FileView{settled, settled}},
			want:   true,
		},
		{
			name:   "a file the setup holds is gone after the repair",
			before: View{State: StateOn, Files: []FileView{settled}},
			after:  View{State: StateOn},
			want:   true,
		},
		{
			name:   "a file that was gone is back after the repair",
			before: View{State: StateOn, Files: []FileView{{Present: false}}},
			after:  View{State: StateOn, Files: []FileView{{Present: true}}},
			want:   true,
		},
		{
			name:   "an editor drifted a file the repair left alone",
			before: View{State: StateOn, Files: []FileView{settled}},
			after:  View{State: StateOn, Files: []FileView{{Kind: KindCodexConfig, Present: true, Drifted: true}}},
			want:   true,
		},
		{
			name:   "the repair rewrote the same files",
			before: View{State: StateOn, Files: []FileView{settled}},
			after:  View{State: StateOn, Files: []FileView{settled}},
			want:   false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := configChanged(tc.before, tc.after); got != tc.want {
				t.Fatalf("configChanged() = %v, want %v", got, tc.want)
			}
		})
	}
}
