package nodes

import (
	"time"
)

type ServiceUnlockStatus string

const (
	StatusUnlocked ServiceUnlockStatus = "unlocked" // 解锁可用
	StatusBlocked  ServiceUnlockStatus = "blocked"  // 风控或地区封锁
	StatusUnknown  ServiceUnlockStatus = "unknown"  // 探测超时或未知
)

type UnlockResult struct {
	IP            string              `json:"ip"`
	OpenAI        ServiceUnlockStatus `json:"openai"`         // ChatGPT / OpenAI
	Claude        ServiceUnlockStatus `json:"claude"`         // Claude / Anthropic
	Google        ServiceUnlockStatus `json:"google"`         // Google Search / 204
	Netflix       ServiceUnlockStatus `json:"netflix"`        // Netflix
	NetflixRegion string              `json:"netflix_region"` // 地区代码，如 "JP", "US"
	CheckedAt     time.Time           `json:"checked_at"`
}

// MatchFilter checks whether the unlock status satisfies the requested filter criteria.
func (u *UnlockResult) MatchFilter(filter string) bool {
	if filter == "" || filter == "none" || filter == "all_unlimited" {
		return true
	}
	if u == nil {
		return false
	}
	switch filter {
	case "ai":
		return u.OpenAI == StatusUnlocked || u.Claude == StatusUnlocked
	case "openai":
		return u.OpenAI == StatusUnlocked
	case "claude":
		return u.Claude == StatusUnlocked
	case "streaming":
		return u.Netflix == StatusUnlocked || u.Google == StatusUnlocked
	case "netflix":
		return u.Netflix == StatusUnlocked
	case "full", "both", "all":
		hasAI := (u.OpenAI == StatusUnlocked || u.Claude == StatusUnlocked)
		hasStreaming := (u.Netflix == StatusUnlocked || u.Google == StatusUnlocked)
		return hasAI && hasStreaming
	default:
		return true
	}
}
