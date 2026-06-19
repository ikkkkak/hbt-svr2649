package meskenyguide

import (
	"strings"
	"sync"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"
)

type localeEntry struct {
	lang string
	at   time.Time
}

var (
	localeMu  sync.Mutex
	localeMem = map[uint]localeEntry{}
)

func isDeadlockErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "deadlock detected")
}

// SyncHostLocaleAsync updates notification_preferences.language without blocking HTTP handlers.
// Dedupes in-memory for 30m and skips UPDATE when the value is already correct.
func SyncHostLocaleAsync(userID uint, rawLocale string) {
	loc := normalizeLocale(rawLocale)
	if loc == "" || userID == 0 {
		return
	}

	localeMu.Lock()
	if e, ok := localeMem[userID]; ok && e.lang == loc && time.Since(e.at) < 30*time.Minute {
		localeMu.Unlock()
		return
	}
	localeMu.Unlock()

	go func() {
		var pref models.NotificationPreference
		err := storage.DB.Select("language").
			Where("user_id = ?", userID).
			Order("updated_at DESC").
			Limit(1).
			First(&pref).Error
		if err == nil && normalizeLocale(pref.Language) == loc {
			localeMu.Lock()
			localeMem[userID] = localeEntry{lang: loc, at: time.Now()}
			localeMu.Unlock()
			return
		}

		for attempt := 0; attempt < 3; attempt++ {
			res := storage.DB.Model(&models.NotificationPreference{}).
				Where("user_id = ?", userID).
				Update("language", loc)
			if res.Error == nil {
				break
			}
			if !isDeadlockErr(res.Error) {
				break
			}
			time.Sleep(time.Duration(25*(attempt+1)) * time.Millisecond)
		}

		localeMu.Lock()
		localeMem[userID] = localeEntry{lang: loc, at: time.Now()}
		localeMu.Unlock()
	}()
}
