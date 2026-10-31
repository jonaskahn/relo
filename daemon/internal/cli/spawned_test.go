package cli

import "testing"

func TestSpawnedDaemon(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"spawned run", []string{"daemon", "run", "--home", "h", "--startup-log", "p"}, true},
		{"spawned run with equals", []string{"daemon", "run", "--startup-log=p"}, true},
		{"foreground run", []string{"daemon", "run"}, false},
		{"start", []string{"daemon", "start"}, false},
	}
	for _, tc := range cases {
		if got := spawnedDaemon(tc.args); got != tc.want {
			t.Errorf("%s: spawnedDaemon(%v) = %v, want %v", tc.name, tc.args, got, tc.want)
		}
	}
}
