package cronjob

import (
	"sync/atomic"
	"testing"
)

type blockingCronTestJob struct {
	started chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

type panickingCronTestJob struct {
	calls atomic.Int32
}

type countingCronTestJob struct {
	calls atomic.Int32
}

func (j *panickingCronTestJob) Run() {
	j.calls.Add(1)
	panic("test panic")
}

func (j *countingCronTestJob) Run() {
	j.calls.Add(1)
}

func (j *blockingCronTestJob) Run() {
	if j.calls.Add(1) == 1 {
		close(j.started)
		<-j.release
	}
}

func TestNonOverlappingJobSkipsConcurrentRun(t *testing.T) {
	job := &blockingCronTestJob{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	wrapped := wrapNonOverlappingJob("test", job)

	done := make(chan struct{})
	go func() {
		wrapped.Run()
		close(done)
	}()
	<-job.started

	wrapped.Run()
	if got := job.calls.Load(); got != 1 {
		t.Fatalf("concurrent run calls = %d, want 1", got)
	}

	close(job.release)
	<-done
	wrapped.Run()
	if got := job.calls.Load(); got != 2 {
		t.Fatalf("run after completion calls = %d, want 2", got)
	}
}

func TestNonOverlappingJobRecoversAndReleasesRunGuard(t *testing.T) {
	job := &panickingCronTestJob{}
	wrapped := wrapNonOverlappingJob("panic test", job)

	wrapped.Run()
	wrapped.Run()

	if got := job.calls.Load(); got != 2 {
		t.Fatalf("panic recovery calls = %d, want 2", got)
	}
}

func TestCronJobScheduleReloadSharesRunGuard(t *testing.T) {
	scheduler := NewCronJob()
	firstJob := &blockingCronTestJob{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	secondJob := &countingCronTestJob{}
	first := scheduler.wrapNonOverlappingJob("timezone reload test", firstJob)
	second := scheduler.wrapNonOverlappingJob("timezone reload test", secondJob)

	done := make(chan struct{})
	go func() {
		first.Run()
		close(done)
	}()
	<-firstJob.started

	second.Run()
	if got := secondJob.calls.Load(); got != 0 {
		t.Fatalf("replacement scheduler overlapped active job: calls=%d", got)
	}

	close(firstJob.release)
	<-done
	second.Run()
	if got := secondJob.calls.Load(); got != 1 {
		t.Fatalf("replacement scheduler did not run after active job completed: calls=%d", got)
	}
}
