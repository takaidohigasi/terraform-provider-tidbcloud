package provider

import (
	"context"
	"strings"
	"sync"

	"github.com/juju/errors"
	"github.com/tidbcloud/terraform-provider-tidbcloud/tidbcloud"
)

// memberListCache lists the organization members once and serves the member
// lookups of a Terraform run from that list. Without it, every tidbcloud_member
// and tidbcloud_member read calls ListMembers on its own, so a plan with N
// members sends N requests and can exceed the per-API-key rate limit.
//
// The provider process lives for one Terraform command, so the list is as fresh
// as the refresh of that command. Writes to members call invalidate so that a
// later lookup lists the members again.
type memberListCache struct {
	mu      sync.Mutex
	loaded  bool
	members []tidbcloud.OpenApiUser
}

// list returns all members of the organization, listing them on the first
// call. A failed listing is not cached, so the next call tries again.
func (c *memberListCache) list(ctx context.Context, client tidbcloud.TiDBCloudIAMClient) ([]tidbcloud.OpenApiUser, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.loaded {
		return c.members, nil
	}
	members, err := listAllMembers(ctx, client)
	if err != nil {
		return nil, err
	}
	c.members, c.loaded = members, true
	return members, nil
}

// find returns the member with the given email from the cached list, or nil if
// no member has that email.
func (c *memberListCache) find(ctx context.Context, client tidbcloud.TiDBCloudIAMClient, email string) (*tidbcloud.OpenApiUser, error) {
	members, err := c.list(ctx, client)
	if err != nil {
		return nil, err
	}
	for i := range members {
		if members[i].Email != nil && strings.EqualFold(*members[i].Email, email) {
			u := members[i]
			return &u, nil
		}
	}
	return nil, nil
}

func (c *memberListCache) invalidate() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.loaded, c.members = false, nil
}

func listAllMembers(ctx context.Context, client tidbcloud.TiDBCloudIAMClient) ([]tidbcloud.OpenApiUser, error) {
	var items []tidbcloud.OpenApiUser

	pageSize := int32(DefaultPageSize)
	var pageToken *string
	for {
		rsp, err := client.ListMembers(ctx, &tidbcloud.ListMembersParams{
			PageSize:  &pageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, errors.Trace(err)
		}
		items = append(items, rsp.Users...)

		pageToken = rsp.NextPageToken
		if IsNilOrEmpty(pageToken) {
			break
		}
	}
	return items, nil
}
