package handler

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// SubscriptionAccountHandler 处理普通用户的订阅账号只读查询。
type SubscriptionAccountHandler struct {
	service *service.SubscriptionAccountService
}

func NewSubscriptionAccountHandler(service *service.SubscriptionAccountService) *SubscriptionAccountHandler {
	return &SubscriptionAccountHandler{service: service}
}

type userSubscriptionAccountGroup struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Platform string `json:"platform"`
}

type userSubscriptionAccountCapacity struct {
	CurrentConcurrency int      `json:"current_concurrency"`
	Concurrency        int      `json:"concurrency"`
	CurrentWindowCost  *float64 `json:"current_window_cost,omitempty"`
	WindowCostLimit    *float64 `json:"window_cost_limit,omitempty"`
	ActiveSessions     *int     `json:"active_sessions,omitempty"`
	MaxSessions        *int     `json:"max_sessions,omitempty"`
	CurrentRPM         *int     `json:"current_rpm,omitempty"`
	BaseRPM            *int     `json:"base_rpm,omitempty"`
	QuotaUsed          *float64 `json:"quota_used,omitempty"`
	QuotaLimit         *float64 `json:"quota_limit,omitempty"`
	QuotaDailyUsed     *float64 `json:"quota_daily_used,omitempty"`
	QuotaDailyLimit    *float64 `json:"quota_daily_limit,omitempty"`
	QuotaWeeklyUsed    *float64 `json:"quota_weekly_used,omitempty"`
	QuotaWeeklyLimit   *float64 `json:"quota_weekly_limit,omitempty"`
}

type userSubscriptionAccountUsageWindow struct {
	Key         string   `json:"key"`
	Utilization *float64 `json:"utilization,omitempty"`
	ResetsAt    string   `json:"resets_at,omitempty"`
	Used        *int64   `json:"used,omitempty"`
	Limit       *int64   `json:"limit,omitempty"`
}

type userSubscriptionAccount struct {
	ID                     int64                                `json:"id"`
	Name                   string                               `json:"name"`
	Platform               string                               `json:"platform"`
	Type                   string                               `json:"type"`
	Capacity               userSubscriptionAccountCapacity      `json:"capacity"`
	Status                 string                               `json:"status"`
	Schedulable            bool                                 `json:"schedulable"`
	RateLimitResetAt       *time.Time                           `json:"rate_limit_reset_at,omitempty"`
	OverloadUntil          *time.Time                           `json:"overload_until,omitempty"`
	TempUnschedulableUntil *time.Time                           `json:"temp_unschedulable_until,omitempty"`
	TodayStats             *service.WindowStats                 `json:"today_stats,omitempty"`
	Groups                 []userSubscriptionAccountGroup       `json:"groups"`
	UsageWindows           []userSubscriptionAccountUsageWindow `json:"usage_windows"`
	UsageUpdatedAt         *time.Time                           `json:"usage_updated_at,omitempty"`
	RateMultiplier         float64                              `json:"rate_multiplier"`
	LastUsedAt             *time.Time                           `json:"last_used_at,omitempty"`
	CreatedAt              time.Time                            `json:"created_at"`
}

// List 返回当前登录用户有效订阅分组内的账号。
// GET /api/v1/subscription-accounts
func (h *SubscriptionAccountHandler) List(c *gin.Context) {
	subject, ok := middleware.GetAuthSubjectFromContext(c)
	if !ok {
		response.Unauthorized(c, "User not authenticated")
		return
	}
	page, pageSize := response.ParsePagination(c)
	if pageSize > 100 {
		pageSize = 100
	}
	search := strings.TrimSpace(c.Query("search"))
	if len(search) > 100 {
		search = search[:100]
	}

	var groupID int64
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			response.BadRequest(c, "Invalid group ID")
			return
		}
		groupID = value
	}

	result, err := h.service.List(c.Request.Context(), subject.UserID, service.SubscriptionAccountListOptions{
		Page: page, PageSize: pageSize, Search: search, GroupID: groupID,
	})
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	items := make([]userSubscriptionAccount, 0, len(result.Items))
	for i := range result.Items {
		items = append(items, userSubscriptionAccountFromService(&result.Items[i]))
	}
	response.Paginated(c, items, result.Total, result.Page, result.Size)
}

func userSubscriptionAccountFromService(item *service.SubscriptionAccountItem) userSubscriptionAccount {
	account := item.Account
	groups := make([]userSubscriptionAccountGroup, 0, len(item.Groups))
	for _, group := range item.Groups {
		groups = append(groups, userSubscriptionAccountGroup{
			ID: group.ID, Name: group.Name, Platform: group.Platform,
		})
	}

	return userSubscriptionAccount{
		ID:                     account.ID,
		Name:                   account.Name,
		Platform:               account.Platform,
		Type:                   account.Type,
		Capacity:               userSubscriptionAccountCapacityFromService(account, item),
		Status:                 account.Status,
		Schedulable:            account.Schedulable,
		RateLimitResetAt:       account.RateLimitResetAt,
		OverloadUntil:          account.OverloadUntil,
		TempUnschedulableUntil: account.TempUnschedulableUntil,
		TodayStats:             item.TodayStats,
		Groups:                 groups,
		UsageWindows:           userSubscriptionAccountUsageWindows(item.Usage),
		UsageUpdatedAt:         usageUpdatedAt(item.Usage),
		RateMultiplier:         account.BillingRateMultiplier(),
		LastUsedAt:             account.LastUsedAt,
		CreatedAt:              account.CreatedAt,
	}
}

func userSubscriptionAccountCapacityFromService(
	account *service.Account,
	item *service.SubscriptionAccountItem,
) userSubscriptionAccountCapacity {
	capacity := userSubscriptionAccountCapacity{
		CurrentConcurrency: item.CurrentConcurrency,
		Concurrency:        account.Concurrency,
		CurrentWindowCost:  item.CurrentWindowCost,
		ActiveSessions:     item.ActiveSessions,
		CurrentRPM:         item.CurrentRPM,
	}
	setPositiveFloat := func(value float64) *float64 {
		if value <= 0 {
			return nil
		}
		copyValue := value
		return &copyValue
	}
	setNonNegativeFloat := func(value float64) *float64 {
		copyValue := value
		return &copyValue
	}
	setPositiveInt := func(value int) *int {
		if value <= 0 {
			return nil
		}
		copyValue := value
		return &copyValue
	}

	capacity.WindowCostLimit = setPositiveFloat(account.GetWindowCostLimit())
	capacity.MaxSessions = setPositiveInt(account.GetMaxSessions())
	capacity.BaseRPM = setPositiveInt(account.GetBaseRPM())
	capacity.QuotaLimit = setPositiveFloat(account.GetQuotaLimit())
	capacity.QuotaDailyLimit = setPositiveFloat(account.GetQuotaDailyLimit())
	capacity.QuotaWeeklyLimit = setPositiveFloat(account.GetQuotaWeeklyLimit())
	if capacity.QuotaLimit != nil {
		capacity.QuotaUsed = setNonNegativeFloat(account.GetQuotaUsed())
	}
	if capacity.QuotaDailyLimit != nil {
		capacity.QuotaDailyUsed = setNonNegativeFloat(account.GetQuotaDailyUsed())
	}
	if capacity.QuotaWeeklyLimit != nil {
		capacity.QuotaWeeklyUsed = setNonNegativeFloat(account.GetQuotaWeeklyUsed())
	}
	return capacity
}

func usageUpdatedAt(usage *service.UsageInfo) *time.Time {
	if usage == nil {
		return nil
	}
	return usage.UpdatedAt
}

func userSubscriptionAccountUsageWindows(usage *service.UsageInfo) []userSubscriptionAccountUsageWindow {
	windows := make([]userSubscriptionAccountUsageWindow, 0, 12)
	if usage == nil {
		return windows
	}
	appendProgress := func(key string, progress *service.UsageProgress) {
		if progress == nil {
			return
		}
		utilization := progress.Utilization
		window := userSubscriptionAccountUsageWindow{Key: key, Utilization: &utilization}
		if progress.ResetsAt != nil {
			window.ResetsAt = progress.ResetsAt.Format(time.RFC3339)
		}
		if progress.LimitRequests > 0 {
			used, limit := progress.UsedRequests, progress.LimitRequests
			window.Used, window.Limit = &used, &limit
		}
		windows = append(windows, window)
	}
	appendProgress("five_hour", usage.FiveHour)
	appendProgress("seven_day", usage.SevenDay)
	appendProgress("seven_day_sonnet", usage.SevenDaySonnet)
	appendProgress("seven_day_fable", usage.SevenDayFable)
	appendProgress("thirty_day", usage.ThirtyDay)
	appendProgress("gemini_shared_daily", usage.GeminiSharedDaily)
	appendProgress("gemini_pro_daily", usage.GeminiProDaily)
	appendProgress("gemini_flash_daily", usage.GeminiFlashDaily)
	appendProgress("gemini_shared_minute", usage.GeminiSharedMinute)
	appendProgress("gemini_pro_minute", usage.GeminiProMinute)
	appendProgress("gemini_flash_minute", usage.GeminiFlashMinute)

	quotaKeys := make([]string, 0, len(usage.AntigravityQuota))
	for key := range usage.AntigravityQuota {
		quotaKeys = append(quotaKeys, key)
	}
	sort.Strings(quotaKeys)
	for _, key := range quotaKeys {
		quota := usage.AntigravityQuota[key]
		if quota == nil {
			continue
		}
		utilization := float64(quota.Utilization)
		windows = append(windows, userSubscriptionAccountUsageWindow{
			Key: "antigravity:" + key, Utilization: &utilization, ResetsAt: quota.ResetTime,
		})
	}
	appendGrokQuota := func(key string, limit, remaining, resetUnix *int64, resetAt string) {
		if limit == nil && remaining == nil && resetUnix == nil && resetAt == "" {
			return
		}
		window := userSubscriptionAccountUsageWindow{Key: key, Limit: limit, ResetsAt: resetAt}
		if limit != nil && remaining != nil {
			used := *limit - *remaining
			if used < 0 {
				used = 0
			}
			window.Used = &used
			if *limit > 0 {
				utilization := float64(used) / float64(*limit) * 100
				window.Utilization = &utilization
			}
		}
		if window.ResetsAt == "" && resetUnix != nil {
			window.ResetsAt = time.Unix(*resetUnix, 0).Format(time.RFC3339)
		}
		windows = append(windows, window)
	}
	if quota := usage.GrokRequestQuota; quota != nil {
		appendGrokQuota("grok_requests", quota.Limit, quota.Remaining, quota.ResetUnix, quota.ResetAt)
	}
	if quota := usage.GrokTokenQuota; quota != nil {
		appendGrokQuota("grok_tokens", quota.Limit, quota.Remaining, quota.ResetUnix, quota.ResetAt)
	}
	return windows
}
