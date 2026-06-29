package service

import "time"

type clientAccessEvaluation struct {
	Blocked        bool
	Expired        bool
	VolumeExceeded bool
}

func normalizeClientVolume(volume int64) int64 {
	if volume < 0 {
		return 0
	}
	return volume
}

func normalizeClientExpiry(expiry int64) int64 {
	if expiry < 0 {
		return 0
	}
	return expiry
}

func normalizeClientResetDay(resetDay int) int {
	if resetDay < 0 {
		return 0
	}
	if resetDay > 31 {
		return 31
	}
	return resetDay
}

func normalizeClientSpeedLimitMbps(limit int) int {
	if limit <= 0 {
		return 0
	}
	return limit
}

func getClientAccessPolicyLocation() *time.Location {
	loc, err := (&SettingService{}).GetTimeLocation()
	if err != nil || loc == nil {
		return time.Local
	}
	return loc
}

func evaluateClientAccess(enable bool, used int64, volume int64, expiry int64, now int64) clientAccessEvaluation {
	if used < 0 {
		used = 0
	}
	volume = normalizeClientVolume(volume)
	expiry = normalizeClientExpiry(expiry)

	volumeExceeded := volume > 0 && used >= volume
	expired := expiry > 0 && now >= expiry

	return clientAccessEvaluation{
		Blocked:        enable && (expired || volumeExceeded),
		Expired:        expired,
		VolumeExceeded: volumeExceeded,
	}
}

func computeClientMonthlyResetBoundary(resetDay int, year int, month time.Month, loc *time.Location) time.Time {
	if resetDay <= 0 {
		return time.Time{}
	}
	effectiveDay := clampResetDayToMonthEnd(resetDay, year, month, loc)
	return time.Date(year, month, effectiveDay, 0, 0, 0, 0, loc)
}

func latestClientMonthlyResetBoundary(resetDay int, now time.Time) (time.Time, bool) {
	resetDay = normalizeClientResetDay(resetDay)
	if resetDay <= 0 {
		return time.Time{}, false
	}

	loc := now.Location()
	currentBoundary := computeClientMonthlyResetBoundary(resetDay, now.Year(), now.Month(), loc)
	if !now.Before(currentBoundary) {
		return currentBoundary, true
	}

	firstOfCurrentMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
	previousMonth := firstOfCurrentMonth.AddDate(0, -1, 0)
	return computeClientMonthlyResetBoundary(resetDay, previousMonth.Year(), previousMonth.Month(), loc), true
}

func nextClientMonthlyResetBoundary(resetDay int, now time.Time) (time.Time, bool) {
	resetDay = normalizeClientResetDay(resetDay)
	if resetDay <= 0 {
		return time.Time{}, false
	}

	loc := now.Location()
	currentBoundary := computeClientMonthlyResetBoundary(resetDay, now.Year(), now.Month(), loc)
	if now.Before(currentBoundary) {
		return currentBoundary, true
	}

	firstOfNextMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, 1, 0)
	return computeClientMonthlyResetBoundary(resetDay, firstOfNextMonth.Year(), firstOfNextMonth.Month(), loc), true
}

func shouldResetClientTrafficMonthly(lastReset int64, resetDay int, now time.Time) bool {
	boundary, ok := latestClientMonthlyResetBoundary(resetDay, now)
	if !ok || boundary.IsZero() {
		return false
	}
	return lastReset < boundary.Unix() && !now.Before(boundary)
}
