package service

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// SubscriptionAccountGroup 是当前用户有效订阅分组的只读概要。
type SubscriptionAccountGroup struct {
	ID       int64
	Name     string
	Platform string
}

// SubscriptionAccountItem 聚合订阅账号页面所需的账号及全局运行数据。
type SubscriptionAccountItem struct {
	Account            *Account
	Groups             []SubscriptionAccountGroup
	CurrentConcurrency int
	CurrentWindowCost  *float64
	ActiveSessions     *int
	CurrentRPM         *int
	TodayStats         *WindowStats
	Usage              *UsageInfo
}

type SubscriptionAccountListOptions struct {
	Page     int
	PageSize int
	Search   string
	GroupID  int64
}

type SubscriptionAccountListResult struct {
	Items  []SubscriptionAccountItem
	Groups []SubscriptionAccountGroup
	Total  int64
	Page   int
	Size   int
}

// SubscriptionAccountService 为普通用户提供订阅分组账号的只读聚合查询。
type SubscriptionAccountService struct {
	userSubRepo       UserSubscriptionRepository
	accountRepo       AccountRepository
	usageService      *AccountUsageService
	concurrency       *ConcurrencyService
	sessionLimitCache SessionLimitCache
	rpmCache          RPMCache
}

func NewSubscriptionAccountService(
	userSubRepo UserSubscriptionRepository,
	accountRepo AccountRepository,
	usageService *AccountUsageService,
	concurrency *ConcurrencyService,
	sessionLimitCache SessionLimitCache,
	rpmCache RPMCache,
) *SubscriptionAccountService {
	return &SubscriptionAccountService{
		userSubRepo:       userSubRepo,
		accountRepo:       accountRepo,
		usageService:      usageService,
		concurrency:       concurrency,
		sessionLimitCache: sessionLimitCache,
		rpmCache:          rpmCache,
	}
}

func (s *SubscriptionAccountService) List(
	ctx context.Context,
	userID int64,
	opts SubscriptionAccountListOptions,
) (*SubscriptionAccountListResult, error) {
	page, pageSize := normalizeSubscriptionAccountPagination(opts.Page, opts.PageSize)
	subs, err := s.userSubRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}

	groups := make([]SubscriptionAccountGroup, 0, len(subs))
	allowedGroups := make(map[int64]SubscriptionAccountGroup, len(subs))
	for i := range subs {
		group := subs[i].Group
		if group == nil || !group.IsActive() || !group.IsSubscriptionType() {
			continue
		}
		if _, exists := allowedGroups[group.ID]; exists {
			continue
		}
		ref := SubscriptionAccountGroup{ID: group.ID, Name: group.Name, Platform: group.Platform}
		allowedGroups[group.ID] = ref
		groups = append(groups, ref)
	}
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Name == groups[j].Name {
			return groups[i].ID < groups[j].ID
		}
		return strings.ToLower(groups[i].Name) < strings.ToLower(groups[j].Name)
	})

	if opts.GroupID > 0 {
		if group, ok := allowedGroups[opts.GroupID]; ok {
			groups = []SubscriptionAccountGroup{group}
		} else {
			groups = nil
		}
	}

	accountByID := make(map[int64]*Account)
	groupsByAccount := make(map[int64][]SubscriptionAccountGroup)
	for _, group := range groups {
		accounts, listErr := s.accountRepo.ListAllWithFilters(ctx, "", "", "", "", group.ID, "")
		if listErr != nil {
			return nil, listErr
		}
		for i := range accounts {
			account := accounts[i]
			if _, exists := accountByID[account.ID]; !exists {
				copyAccount := account
				accountByID[account.ID] = &copyAccount
			}
			groupsByAccount[account.ID] = appendSubscriptionAccountGroup(groupsByAccount[account.ID], group)
		}
	}

	search := strings.ToLower(strings.TrimSpace(opts.Search))
	accounts := make([]*Account, 0, len(accountByID))
	for _, account := range accountByID {
		if search != "" && !subscriptionAccountMatches(account, groupsByAccount[account.ID], search) {
			continue
		}
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		left, right := strings.ToLower(accounts[i].Name), strings.ToLower(accounts[j].Name)
		if left == right {
			return accounts[i].ID < accounts[j].ID
		}
		return left < right
	})

	total := len(accounts)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	pageAccounts := accounts[start:end]

	items := make([]SubscriptionAccountItem, len(pageAccounts))
	for i, account := range pageAccounts {
		items[i] = SubscriptionAccountItem{
			Account: account,
			Groups:  groupsByAccount[account.ID],
		}
	}
	s.enrichRuntime(ctx, items)

	return &SubscriptionAccountListResult{
		Items:  items,
		Groups: groups,
		Total:  int64(total),
		Page:   page,
		Size:   pageSize,
	}, nil
}

func normalizeSubscriptionAccountPagination(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func appendSubscriptionAccountGroup(
	groups []SubscriptionAccountGroup,
	group SubscriptionAccountGroup,
) []SubscriptionAccountGroup {
	for _, existing := range groups {
		if existing.ID == group.ID {
			return groups
		}
	}
	return append(groups, group)
}

func subscriptionAccountMatches(account *Account, groups []SubscriptionAccountGroup, search string) bool {
	if strings.Contains(strings.ToLower(account.Name), search) ||
		strings.Contains(strings.ToLower(account.Platform), search) ||
		strings.Contains(strings.ToLower(account.Type), search) {
		return true
	}
	for _, group := range groups {
		if strings.Contains(strings.ToLower(group.Name), search) {
			return true
		}
	}
	return false
}

func (s *SubscriptionAccountService) enrichRuntime(ctx context.Context, items []SubscriptionAccountItem) {
	if len(items) == 0 {
		return
	}
	accountIDs := make([]int64, len(items))
	indexByID := make(map[int64]int, len(items))
	for i := range items {
		accountIDs[i] = items[i].Account.ID
		indexByID[items[i].Account.ID] = i
	}

	if s.concurrency != nil {
		if counts, err := s.concurrency.GetAccountConcurrencyBatch(ctx, accountIDs); err == nil {
			for id, count := range counts {
				if index, ok := indexByID[id]; ok {
					items[index].CurrentConcurrency = count
				}
			}
		}
	}

	if s.usageService != nil {
		if stats, err := s.usageService.GetTodayStatsBatch(ctx, accountIDs); err == nil {
			for id, value := range stats {
				if index, ok := indexByID[id]; ok {
					items[index].TodayStats = value
				}
			}
		}
		if usage, _, err := s.usageService.GetUsageBatch(ctx, accountIDs, false); err == nil {
			for id, value := range usage {
				if index, ok := indexByID[id]; ok {
					items[index].Usage = value
				}
			}
		}
	}

	s.enrichCapacityRuntime(ctx, items, indexByID)
}

func (s *SubscriptionAccountService) enrichCapacityRuntime(
	ctx context.Context,
	items []SubscriptionAccountItem,
	indexByID map[int64]int,
) {
	windowAccountIDs := make([]int64, 0)
	sessionAccountIDs := make([]int64, 0)
	rpmAccountIDs := make([]int64, 0)
	idleTimeouts := make(map[int64]time.Duration)

	for i := range items {
		account := items[i].Account
		if !account.IsAnthropicOAuthOrSetupToken() {
			continue
		}
		if account.GetWindowCostLimit() > 0 {
			windowAccountIDs = append(windowAccountIDs, account.ID)
		}
		if account.GetMaxSessions() > 0 {
			sessionAccountIDs = append(sessionAccountIDs, account.ID)
			idleTimeouts[account.ID] = time.Duration(account.GetSessionIdleTimeoutMinutes()) * time.Minute
		}
		if account.GetBaseRPM() > 0 {
			rpmAccountIDs = append(rpmAccountIDs, account.ID)
		}
	}

	if s.sessionLimitCache != nil && len(sessionAccountIDs) > 0 {
		if counts, err := s.sessionLimitCache.GetActiveSessionCountBatch(ctx, sessionAccountIDs, idleTimeouts); err == nil {
			for id, count := range counts {
				if index, ok := indexByID[id]; ok {
					value := count
					items[index].ActiveSessions = &value
				}
			}
		}
	}
	if s.rpmCache != nil && len(rpmAccountIDs) > 0 {
		if counts, err := s.rpmCache.GetRPMBatch(ctx, rpmAccountIDs); err == nil {
			for id, count := range counts {
				if index, ok := indexByID[id]; ok {
					value := count
					items[index].CurrentRPM = &value
				}
			}
		}
	}
	if s.usageService == nil || len(windowAccountIDs) == 0 {
		return
	}

	var mu sync.Mutex
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(8)
	for _, accountID := range windowAccountIDs {
		id := accountID
		index := indexByID[id]
		account := items[index].Account
		group.Go(func() error {
			stats, err := s.usageService.GetAccountWindowStats(groupCtx, id, account.GetCurrentWindowStartTime())
			if err != nil || stats == nil {
				return nil
			}
			value := stats.StandardCost
			mu.Lock()
			items[index].CurrentWindowCost = &value
			mu.Unlock()
			return nil
		})
	}
	_ = group.Wait()
}
