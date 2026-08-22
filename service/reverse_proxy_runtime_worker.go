package service

import (
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
)

const reverseProxyRuntimeMonitorInterval = 30 * time.Second

// reverseProxyRuntimeWorker keeps reverse-proxy reconciliation independent
// from the sampler that handles traffic counters and nftables.  Reconcile
// passes are serialized here, so a slow DNS or listener transition cannot
// delay unrelated runtime maintenance.
type reverseProxyRuntimeWorker struct {
	mu sync.Mutex

	running    bool
	stopping   bool
	stopCh     chan struct{}
	doneCh     chan struct{}
	completeCh chan struct{}
	wakeCh     chan struct{}

	service  *ReverseProxyService
	syncFunc func(*ReverseProxyService) error

	databasePaused       bool
	resumeAfterDBRestore bool
}

var reverseProxyRuntimeMonitor = &reverseProxyRuntimeWorker{}

func init() {
	database.RegisterDBBeforeRestoreHook(PauseReverseProxyRuntimeForDatabaseRestore)
	database.RegisterDBRestoreAbortHook(ResumeReverseProxyRuntimeAfterDatabaseRestoreFailure)
}

// Start starts the coalescing reverse-proxy monitor.  The initial runtime is
// already built synchronously by ReverseProxyService.StartRuntime; this worker
// only handles later revision/certificate changes and periodic maintenance.
func (w *reverseProxyRuntimeWorker) Start(service *ReverseProxyService) {
	if w == nil {
		return
	}
	if service == nil {
		service = &ReverseProxyService{}
	}
	for {
		w.mu.Lock()
		if w.running {
			w.service = service
			wakeCh := w.wakeCh
			w.mu.Unlock()
			select {
			case wakeCh <- struct{}{}:
			default:
			}
			return
		}
		if w.stopping {
			completeCh := w.completeCh
			w.mu.Unlock()
			if completeCh != nil {
				<-completeCh
			}
			continue
		}
		if w.databasePaused {
			w.service = service
			w.mu.Unlock()
			return
		}

		w.service = service
		w.running = true
		w.stopCh = make(chan struct{})
		w.doneCh = make(chan struct{})
		w.completeCh = make(chan struct{})
		w.wakeCh = make(chan struct{}, 1)
		stopCh := w.stopCh
		wakeCh := w.wakeCh
		doneCh := w.doneCh
		w.mu.Unlock()

		go w.run(stopCh, wakeCh, doneCh)
		return
	}
}

func (w *reverseProxyRuntimeWorker) AllowAfterDatabaseRestore() {
	if w == nil {
		return
	}
	w.mu.Lock()
	w.databasePaused = false
	w.resumeAfterDBRestore = false
	w.mu.Unlock()
}

func (w *reverseProxyRuntimeWorker) StopAndWait() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.running && !w.stopping {
		w.mu.Unlock()
		return
	}
	if w.stopping {
		completeCh := w.completeCh
		w.mu.Unlock()
		if completeCh != nil {
			<-completeCh
		}
		return
	}

	doneCh := w.doneCh
	completeCh := w.completeCh
	stopCh := w.stopCh
	w.running = false
	w.stopping = true
	if stopCh != nil {
		close(stopCh)
	}
	w.mu.Unlock()

	if doneCh != nil {
		<-doneCh
	}

	w.mu.Lock()
	if w.doneCh == doneCh {
		w.stopCh = nil
		w.doneCh = nil
		w.completeCh = nil
		w.wakeCh = nil
		w.stopping = false
		if completeCh != nil {
			close(completeCh)
		}
	}
	w.mu.Unlock()
}

// Wake requests one immediate reconciliation.  The buffered channel merges
// repeated certificate/configuration notifications into one bounded pass.
func (w *reverseProxyRuntimeWorker) Wake() {
	if w == nil {
		return
	}
	w.mu.Lock()
	wakeCh := w.wakeCh
	running := w.running
	w.mu.Unlock()
	if !running || wakeCh == nil {
		return
	}
	select {
	case wakeCh <- struct{}{}:
	default:
	}
}

// PauseForDatabaseRestore closes the worker before SQLite is closed or
// replaced.  A failed restore can resume only a worker that was active before
// the restore attempt.
func (w *reverseProxyRuntimeWorker) PauseForDatabaseRestore() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	if w.databasePaused {
		w.mu.Unlock()
		return nil
	}
	wasRunning := w.running
	wasActive := w.running || w.stopping
	w.databasePaused = true
	w.resumeAfterDBRestore = wasRunning
	w.mu.Unlock()
	if wasActive {
		w.StopAndWait()
	}
	return nil
}

func (w *reverseProxyRuntimeWorker) ResumeAfterDatabaseRestoreFailure() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.databasePaused {
		w.mu.Unlock()
		return
	}
	shouldResume := w.resumeAfterDBRestore
	service := w.service
	w.databasePaused = false
	w.resumeAfterDBRestore = false
	w.mu.Unlock()
	if shouldResume {
		w.Start(service)
	}
}

func (w *reverseProxyRuntimeWorker) run(stopCh <-chan struct{}, wakeCh <-chan struct{}, doneCh chan<- struct{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("reverse proxy runtime monitor panicked: ", recovered)
		}
		w.mu.Lock()
		if w.doneCh == doneCh {
			w.running = false
			if !w.stopping {
				w.stopCh = nil
				w.doneCh = nil
				completeCh := w.completeCh
				w.completeCh = nil
				w.wakeCh = nil
				if completeCh != nil {
					close(completeCh)
				}
			}
		}
		w.mu.Unlock()
		close(doneCh)
	}()

	ticker := time.NewTicker(reverseProxyRuntimeMonitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-wakeCh:
			w.runSyncSafely()
		case <-ticker.C:
			w.runSyncSafely()
		}
	}
}

func (w *reverseProxyRuntimeWorker) runSyncSafely() {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("reverse proxy runtime monitor sync panicked: ", recovered)
		}
	}()
	w.runSync()
}

func (w *reverseProxyRuntimeWorker) runSync() {
	if w == nil {
		return
	}
	w.mu.Lock()
	service := w.service
	syncFunc := w.syncFunc
	w.mu.Unlock()
	if service == nil {
		return
	}
	operation, err := BeginKworInProcessOperation("reverse-proxy-runtime-monitor")
	if err != nil {
		logger.Warning("reverse proxy runtime monitor skipped by lifecycle quiesce: ", err)
		return
	}
	defer operation.Done()
	var syncErr error
	if syncFunc != nil {
		syncErr = syncFunc(service)
	} else {
		syncErr = service.SyncIfNeeded(3 * time.Second)
	}
	if syncErr != nil {
		logger.Warning("reverse proxy runtime monitor failed: ", syncErr)
	}
}

func WakeReverseProxyRuntime() {
	reverseProxyRuntimeMonitor.Wake()
}

func PauseReverseProxyRuntimeForDatabaseRestore() error {
	return reverseProxyRuntimeMonitor.PauseForDatabaseRestore()
}

func ResumeReverseProxyRuntimeAfterDatabaseRestoreFailure() {
	reverseProxyRuntimeMonitor.ResumeAfterDatabaseRestoreFailure()
}
