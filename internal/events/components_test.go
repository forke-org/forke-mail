package events

import (
	"testing"
	"time"
)

func TestComponentJudgement(t *testing.T) {
	th := threshold{time.Minute, 10 * time.Minute}
	for lag, want := range map[float64]string{5: "operational", 120: "degraded", 900: "down"} {
		if got := th.judge(&lag); got != want {
			t.Fatalf("lag %v: %s, want %s", lag, got, want)
		}
	}
	if th.judge(nil) != "operational" {
		t.Fatal("idle component is not operational")
	}
	if got := worst([]Component{{Status: "degraded"}, {Status: "not_enabled"}, {Status: "operational"}}); got != "degraded" {
		t.Fatal(got)
	}
}
