package cronjob

import "testing"

func TestStatsJobRunGuard(t *testing.T) {
	job := NewStatsJob(false)

	if !job.beginRun() {
		t.Fatal("first stats job run should start")
	}
	if job.beginRun() {
		t.Fatal("overlapping stats job run should be skipped")
	}

	job.finishRun()
	if !job.beginRun() {
		t.Fatal("stats job should run again after completion")
	}
	job.finishRun()
}
