package cronjob

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
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

func TestStopCronSchedulerWaitsForRunningJob(t *testing.T) {
	scheduler := cron.New(cron.WithSeconds())
	job := &blockingCronTestJob{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	if _, err := scheduler.AddJob("@every 1s", job); err != nil {
		t.Fatalf("add blocking cron job failed: %v", err)
	}
	scheduler.Start()

	select {
	case <-job.started:
	case <-time.After(3 * time.Second):
		scheduler.Stop()
		t.Fatal("scheduled test job did not start")
	}

	stopped := make(chan struct{})
	go func() {
		stopCronSchedulerAndWait(scheduler)
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("cron scheduler stopped before its running job completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(job.release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("cron scheduler did not finish after its running job completed")
	}
}

func TestCronJobStopWaitsForImmediateJob(t *testing.T) {
	c := NewCronJob()
	job := &blockingCronTestJob{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	c.runImmediateJob(job)
	select {
	case <-job.started:
	case <-time.After(time.Second):
		t.Fatal("immediate test job did not start")
	}

	stopped := make(chan struct{})
	go func() {
		c.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("CronJob.Stop returned before immediate job completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(job.release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("CronJob.Stop did not finish after immediate job completed")
	}
}
