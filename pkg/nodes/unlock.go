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
