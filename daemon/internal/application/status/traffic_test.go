package status

import (
	"context"
	"errors"
	"testing"
)

type fakeTraffic struct {
	on    bool
	stats TrafficStats
	err   error
}

func (f fakeTraffic) On() bool { return f.on }

func (f fakeTraffic) Stats() (TrafficStats, error) { return f.stats, f.err }

func TestTrafficCheckOff(t *testing.T) {
	check := (&Service{traffic: fakeTraffic{}}).checkTraffic(context.Background())
	if check.Status != CheckPass || check.Detail != "off" {
		t.Fatalf("checkTraffic(off) = %+v, want a pass saying off", check)
	}
}

func TestTrafficCheckUnderTheWarningLine(t *testing.T) {
	check := (&Service{traffic: fakeTraffic{on: true, stats: TrafficStats{TodayBytes: 10, TotalBytes: 20}}}).checkTraffic(context.Background())
	if check.Status != CheckPass || check.Detail != "today 10 B, 20 B retained" {
		t.Fatalf("checkTraffic(on) = %+v, want the sizes and a pass", check)
	}
}

func TestTrafficCheckOverTheWarningLine(t *testing.T) {
	check := (&Service{traffic: fakeTraffic{on: true, stats: TrafficStats{TotalBytes: 1<<30 + 1}}}).checkTraffic(context.Background())
	if check.Status != CheckWarn {
		t.Fatalf("checkTraffic(over) = %+v, want a warning", check)
	}
}

func TestTrafficCheckFailsWhenStatsCannotBeRead(t *testing.T) {
	check := (&Service{traffic: fakeTraffic{on: true, err: errors.New("unreadable")}}).checkTraffic(context.Background())
	if check.Status != CheckFail || check.Detail != "unreadable" {
		t.Fatalf("checkTraffic(error) = %+v, want a failure carrying the reason", check)
	}
}
