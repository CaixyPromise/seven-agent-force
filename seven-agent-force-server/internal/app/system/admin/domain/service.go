package domain

import (
	"sort"
	"strings"
	"time"
)

type Service struct{}

func NewService() *Service {
	return &Service{}
}

func (s *Service) FilterOnlineUsers(items []OnlineUser, username, loginIP, browser, os string) []OnlineUser {
	username = strings.ToLower(strings.TrimSpace(username))
	loginIP = strings.ToLower(strings.TrimSpace(loginIP))
	browser = strings.ToLower(strings.TrimSpace(browser))
	os = strings.ToLower(strings.TrimSpace(os))
	result := make([]OnlineUser, 0, len(items))
	for _, item := range items {
		if username != "" && !strings.Contains(strings.ToLower(item.Username), username) && !strings.Contains(strings.ToLower(item.Nickname), username) {
			continue
		}
		if loginIP != "" && !strings.Contains(strings.ToLower(item.LoginIP), loginIP) {
			continue
		}
		if browser != "" && !strings.Contains(strings.ToLower(item.Browser), browser) {
			continue
		}
		if os != "" && !strings.Contains(strings.ToLower(item.OS), os) {
			continue
		}
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := timeValue(result[i].LastActiveTime)
		right := timeValue(result[j].LastActiveTime)
		if left.Equal(right) {
			return result[i].UserID < result[j].UserID
		}
		return left.After(right)
	})
	return result
}

func (s *Service) PaginateOnlineUsers(items []OnlineUser, current, size int64) ([]OnlineUser, int64) {
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 10
	}
	total := int64(len(items))
	offset := (current - 1) * size
	if offset >= total {
		return []OnlineUser{}, total
	}
	end := offset + size
	if end > total {
		end = total
	}
	return items[offset:end], total
}

func (s *Service) BuildOnlineUserStats(items []OnlineUser) OnlineUserStats {
	stats := OnlineUserStats{
		TotalOnlineUsers: int64(len(items)),
		BrowserStats:     map[string]int64{},
		OSStats:          map[string]int64{},
	}
	now := time.Now().UTC()
	todayThreshold := now.Add(-24 * time.Hour)
	activeThreshold := now.Add(-time.Hour)
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.UserRole), "admin") {
			stats.AdminUsers++
		} else {
			stats.NormalUsers++
		}
		if browser := strings.TrimSpace(item.Browser); browser != "" {
			stats.BrowserStats[browser]++
		}
		if os := strings.TrimSpace(item.OS); os != "" {
			stats.OSStats[os]++
		}
		if item.LoginTime != nil && !item.LoginTime.Before(todayThreshold) {
			stats.TodayLoginUsers++
		}
		if item.LastActiveTime != nil && !item.LastActiveTime.Before(activeThreshold) {
			stats.ActiveUsers++
		}
	}
	stats.TotalOnline = stats.TotalOnlineUsers
	stats.TodayLogin = stats.TodayLoginUsers
	stats.PeakOnline = stats.TotalOnlineUsers
	return stats
}

func timeValue(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}
