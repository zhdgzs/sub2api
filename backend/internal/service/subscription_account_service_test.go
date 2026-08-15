package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type subscriptionAccountUserSubRepoStub struct {
	UserSubscriptionRepository
	subs []UserSubscription
}

func (s *subscriptionAccountUserSubRepoStub) ListActiveByUserID(context.Context, int64) ([]UserSubscription, error) {
	return s.subs, nil
}

type subscriptionAccountRepoStub struct {
	AccountRepository
	byGroup map[int64][]Account
	calls   []int64
}

func (s *subscriptionAccountRepoStub) ListAllWithFilters(
	_ context.Context,
	_, _, _, _ string,
	groupID int64,
	_ string,
) ([]Account, error) {
	s.calls = append(s.calls, groupID)
	return append([]Account(nil), s.byGroup[groupID]...), nil
}

func TestSubscriptionAccountServiceListFiltersAndDeduplicates(t *testing.T) {
	subscriptionGroupA := &Group{ID: 10, Name: "Pro A", Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	subscriptionGroupB := &Group{ID: 20, Name: "Pro B", Platform: PlatformAnthropic, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	standardGroup := &Group{ID: 30, Name: "Public", Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeStandard}
	inactiveGroup := &Group{ID: 40, Name: "Old", Platform: PlatformOpenAI, Status: StatusDisabled, SubscriptionType: SubscriptionTypeSubscription}

	subRepo := &subscriptionAccountUserSubRepoStub{subs: []UserSubscription{
		{GroupID: 10, Group: subscriptionGroupA},
		{GroupID: 20, Group: subscriptionGroupB},
		{GroupID: 30, Group: standardGroup},
		{GroupID: 40, Group: inactiveGroup},
	}}
	accountRepo := &subscriptionAccountRepoStub{byGroup: map[int64][]Account{
		10: {{ID: 1, Name: "Shared", Platform: PlatformOpenAI}, {ID: 2, Name: "Only A", Platform: PlatformOpenAI}},
		20: {{ID: 1, Name: "Shared", Platform: PlatformOpenAI}},
		30: {{ID: 3, Name: "Must not leak", Platform: PlatformOpenAI}},
		40: {{ID: 4, Name: "Inactive", Platform: PlatformOpenAI}},
	}}

	svc := NewSubscriptionAccountService(subRepo, accountRepo, nil, nil, nil, nil)
	result, err := svc.List(context.Background(), 7, SubscriptionAccountListOptions{Page: 1, PageSize: 20})

	require.NoError(t, err)
	require.Equal(t, int64(2), result.Total)
	require.Equal(t, []int64{10, 20}, accountRepo.calls)
	require.Len(t, result.Items, 2)
	require.Equal(t, "Only A", result.Items[0].Account.Name)
	require.Equal(t, "Shared", result.Items[1].Account.Name)
	require.Equal(t, []SubscriptionAccountGroup{
		{ID: 10, Name: "Pro A", Platform: PlatformOpenAI},
		{ID: 20, Name: "Pro B", Platform: PlatformAnthropic},
	}, result.Items[1].Groups)
}

func TestSubscriptionAccountServiceListRejectsUnsubscribedGroupFilter(t *testing.T) {
	group := &Group{ID: 10, Name: "Pro", Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	subRepo := &subscriptionAccountUserSubRepoStub{subs: []UserSubscription{{GroupID: 10, Group: group}}}
	accountRepo := &subscriptionAccountRepoStub{byGroup: map[int64][]Account{10: {{ID: 1, Name: "Visible"}}}}
	svc := NewSubscriptionAccountService(subRepo, accountRepo, nil, nil, nil, nil)

	result, err := svc.List(context.Background(), 7, SubscriptionAccountListOptions{GroupID: 999})

	require.NoError(t, err)
	require.Empty(t, result.Items)
	require.Empty(t, accountRepo.calls)
}

func TestSubscriptionAccountServiceListSearchAndPagination(t *testing.T) {
	group := &Group{ID: 10, Name: "Pro", Platform: PlatformOpenAI, Status: StatusActive, SubscriptionType: SubscriptionTypeSubscription}
	subRepo := &subscriptionAccountUserSubRepoStub{subs: []UserSubscription{{GroupID: 10, Group: group}}}
	accountRepo := &subscriptionAccountRepoStub{byGroup: map[int64][]Account{
		10: {
			{ID: 1, Name: "Alpha", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
			{ID: 2, Name: "Beta", Platform: PlatformAnthropic, Type: AccountTypeAPIKey},
			{ID: 3, Name: "Gamma", Platform: PlatformOpenAI, Type: AccountTypeOAuth},
		},
	}}
	svc := NewSubscriptionAccountService(subRepo, accountRepo, nil, nil, nil, nil)

	result, err := svc.List(context.Background(), 7, SubscriptionAccountListOptions{
		Page: 2, PageSize: 1, Search: "openai",
	})

	require.NoError(t, err)
	require.Equal(t, int64(2), result.Total)
	require.Len(t, result.Items, 1)
	require.Equal(t, "Gamma", result.Items[0].Account.Name)
}
