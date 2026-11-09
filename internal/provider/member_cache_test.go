package provider

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/golang/mock/gomock"
	mockClient "github.com/tidbcloud/terraform-provider-tidbcloud/mock"
	"github.com/tidbcloud/terraform-provider-tidbcloud/tidbcloud"
)

// expectTwoPages makes ListMembers return a, b on the first page and c on the
// second, `times` times in all.
func expectTwoPages(s *mockClient.MockTiDBCloudIAMClient, times int) {
	a := testUTMember("a@example.com", "user-a", "org:member", "Active")
	b := testUTMember("b@example.com", "user-b", "org:owner", "Active")
	c := testUTMember("c@example.com", "user-c", "org:member", "Pending")
	s.EXPECT().ListMembers(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, p *tidbcloud.ListMembersParams) (*tidbcloud.OpenApiListUsersRsp, error) {
			if p.Email != nil {
				return nil, errors.New("the cache must not filter by email")
			}
			if p.PageToken == nil {
				return &tidbcloud.OpenApiListUsersRsp{Users: []tidbcloud.OpenApiUser{a, b}, NextPageToken: Ptr("page-2")}, nil
			}
			return &tidbcloud.OpenApiListUsersRsp{Users: []tidbcloud.OpenApiUser{c}}, nil
		}).Times(2 * times)
}

func TestMemberListCache_ListsOnceForAllLookups(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mockClient.NewMockTiDBCloudIAMClient(ctrl)
	expectTwoPages(s, 1)

	cache := &memberListCache{}
	emails := []string{"a@example.com", "B@example.com", "c@example.com", "missing@example.com"}
	want := []string{"user-a", "user-b", "user-c", ""}
	got := make([]string, len(emails))
	var wg sync.WaitGroup
	for i, email := range emails {
		wg.Add(1)
		go func() {
			defer wg.Done()
			member, err := cache.find(context.Background(), s, email)
			if err != nil {
				t.Errorf("find(%s): %v", email, err)
				return
			}
			if member != nil {
				got[i] = *member.UserId
			}
		}()
	}
	wg.Wait()
	for i := range emails {
		if got[i] != want[i] {
			t.Errorf("find(%s) = %q, want %q", emails[i], got[i], want[i])
		}
	}

	members, err := cache.list(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	if len(members) != 3 {
		t.Errorf("list returned %d members, want 3", len(members))
	}
}

func TestMemberListCache_InvalidateListsAgain(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mockClient.NewMockTiDBCloudIAMClient(ctrl)
	expectTwoPages(s, 2)

	cache := &memberListCache{}
	for i := 0; i < 2; i++ {
		if _, err := cache.find(context.Background(), s, "a@example.com"); err != nil {
			t.Fatal(err)
		}
		cache.invalidate()
	}
}

func TestMemberListCache_DoesNotCacheErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	s := mockClient.NewMockTiDBCloudIAMClient(ctrl)
	gomock.InOrder(
		s.EXPECT().ListMembers(gomock.Any(), gomock.Any()).Return(nil, errors.New("429 Too Many Requests")),
		s.EXPECT().ListMembers(gomock.Any(), gomock.Any()).Return(&tidbcloud.OpenApiListUsersRsp{
			Users: []tidbcloud.OpenApiUser{testUTMember("a@example.com", "user-a", "org:member", "Active")},
		}, nil),
	)

	cache := &memberListCache{}
	if _, err := cache.find(context.Background(), s, "a@example.com"); err == nil {
		t.Fatal("want the listing error")
	}
	member, err := cache.find(context.Background(), s, "a@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if member == nil || *member.UserId != "user-a" {
		t.Errorf("member = %v, want user-a", member)
	}
}
