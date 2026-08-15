package handler

import (
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
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

// userSubscriptionAccountUsage 只保留账号列表用量单元格需要的字段。
// 错误详情、验证链接和上游能力明细不会下放给普通用户。
type userSubscriptionAccountUsage struct {
	Source                 string                                    `json:"source,omitempty"`
	UpdatedAt              *time.Time                                `json:"updated_at,omitempty"`
	FiveHour               *service.UsageProgress                    `json:"five_hour"`
	SevenDay               *service.UsageProgress                    `json:"seven_day,omitempty"`
	SevenDaySonnet         *service.UsageProgress                    `json:"seven_day_sonnet,omitempty"`
	SevenDayFable          *service.UsageProgress                    `json:"seven_day_fable,omitempty"`
	ThirtyDay              *service.UsageProgress                    `json:"thirty_day,omitempty"`
	GeminiSharedDaily      *service.UsageProgress                    `json:"gemini_shared_daily,omitempty"`
	GeminiProDaily         *service.UsageProgress                    `json:"gemini_pro_daily,omitempty"`
	GeminiFlashDaily       *service.UsageProgress                    `json:"gemini_flash_daily,omitempty"`
	GeminiSharedMinute     *service.UsageProgress                    `json:"gemini_shared_minute,omitempty"`
	GeminiProMinute        *service.UsageProgress                    `json:"gemini_pro_minute,omitempty"`
	GeminiFlashMinute      *service.UsageProgress                    `json:"gemini_flash_minute,omitempty"`
	AntigravityQuota       map[string]*service.AntigravityModelQuota `json:"antigravity_quota,omitempty"`
	GrokRequestQuota       *xai.QuotaWindow                          `json:"grok_request_quota,omitempty"`
	GrokTokenQuota         *xai.QuotaWindow                          `json:"grok_token_quota,omitempty"`
	GrokRetryAfterSeconds  *int                                      `json:"grok_retry_after_seconds,omitempty"`
	GrokEntitlementStatus  string                                    `json:"grok_entitlement_status,omitempty"`
	GrokQuotaSnapshotState string                                    `json:"grok_quota_snapshot_state,omitempty"`
	GrokFreeTokenLimit     int64                                     `json:"grok_free_token_limit,omitempty"`
	GrokLocalUsage24h      *service.WindowStats                      `json:"grok_local_usage_24h,omitempty"`
	GrokBilling            *xai.BillingSummary                       `json:"grok_billing,omitempty"`
	SubscriptionTier       string                                    `json:"subscription_tier,omitempty"`
	AICredits              []service.AICredit                        `json:"ai_credits,omitempty"`
	IsForbidden            bool                                      `json:"is_forbidden,omitempty"`
	ForbiddenType          string                                    `json:"forbidden_type,omitempty"`
	NeedsReauth            bool                                      `json:"needs_reauth,omitempty"`
	ErrorCode              string                                    `json:"error_code,omitempty"`
}

type userSubscriptionAccount struct {
	ID                     int64                           `json:"id"`
	Name                   string                          `json:"name"`
	Platform               string                          `json:"platform"`
	Type                   string                          `json:"type"`
	Capacity               userSubscriptionAccountCapacity `json:"capacity"`
	Status                 string                          `json:"status"`
	Schedulable            bool                            `json:"schedulable"`
	RateLimitResetAt       *time.Time                      `json:"rate_limit_reset_at,omitempty"`
	OverloadUntil          *time.Time                      `json:"overload_until,omitempty"`
	TempUnschedulableUntil *time.Time                      `json:"temp_unschedulable_until,omitempty"`
	TodayStats             *service.WindowStats            `json:"today_stats,omitempty"`
	Groups                 []userSubscriptionAccountGroup  `json:"groups"`
	Usage                  *userSubscriptionAccountUsage   `json:"usage,omitempty"`
	RateMultiplier         float64                         `json:"rate_multiplier"`
	LastUsedAt             *time.Time                      `json:"last_used_at,omitempty"`
	CreatedAt              time.Time                       `json:"created_at"`
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
		Usage:                  userSubscriptionAccountUsageFromService(item.Usage),
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

func userSubscriptionAccountUsageFromService(usage *service.UsageInfo) *userSubscriptionAccountUsage {
	if usage == nil {
		return nil
	}
	return &userSubscriptionAccountUsage{
		Source:                 usage.Source,
		UpdatedAt:              usage.UpdatedAt,
		FiveHour:               usage.FiveHour,
		SevenDay:               usage.SevenDay,
		SevenDaySonnet:         usage.SevenDaySonnet,
		SevenDayFable:          usage.SevenDayFable,
		ThirtyDay:              usage.ThirtyDay,
		GeminiSharedDaily:      usage.GeminiSharedDaily,
		GeminiProDaily:         usage.GeminiProDaily,
		GeminiFlashDaily:       usage.GeminiFlashDaily,
		GeminiSharedMinute:     usage.GeminiSharedMinute,
		GeminiProMinute:        usage.GeminiProMinute,
		GeminiFlashMinute:      usage.GeminiFlashMinute,
		AntigravityQuota:       usage.AntigravityQuota,
		GrokRequestQuota:       usage.GrokRequestQuota,
		GrokTokenQuota:         usage.GrokTokenQuota,
		GrokRetryAfterSeconds:  usage.GrokRetryAfterSeconds,
		GrokEntitlementStatus:  usage.GrokEntitlementStatus,
		GrokQuotaSnapshotState: usage.GrokQuotaSnapshotState,
		GrokFreeTokenLimit:     usage.GrokFreeTokenLimit,
		GrokLocalUsage24h:      usage.GrokLocalUsage24h,
		GrokBilling:            usage.GrokBilling,
		SubscriptionTier:       usage.SubscriptionTier,
		AICredits:              usage.AICredits,
		IsForbidden:            usage.IsForbidden,
		ForbiddenType:          usage.ForbiddenType,
		NeedsReauth:            usage.NeedsReauth,
		ErrorCode:              usage.ErrorCode,
	}
}
