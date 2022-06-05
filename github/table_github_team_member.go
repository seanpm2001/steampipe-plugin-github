package github

import (
	"context"

	"github.com/google/go-github/v33/github"

	pb "github.com/turbot/steampipe-plugin-sdk/v3/grpc/proto"
	"github.com/turbot/steampipe-plugin-sdk/v3/plugin"
	"github.com/turbot/steampipe-plugin-sdk/v3/plugin/transform"
)

func gitHubTeamMemberColumns() []*plugin.Column {
	var repoColumns = gitHubUserColumns()
	var teamColumns = []*plugin.Column{
		{Name: "organization", Type: pb.ColumnType_STRING, Description: "The organization the team is associated with.", Transform: transform.FromQual("organization")},
		{Name: "slug", Type: pb.ColumnType_STRING, Description: "The team slug name.", Transform: transform.FromQual("slug")},
		{Name: "role", Type: pb.ColumnType_STRING, Description: "The team member's role", Hydrate: tableGitHubTeamMemberGet},
		{Name: "state", Type: pb.ColumnType_STRING, Description: "The membership state", Hydrate: tableGitHubTeamMemberGet},
	}

	return append(repoColumns, teamColumns...)
}

//// TABLE DEFINITION

func tableGitHubTeamMember() *plugin.Table {
	return &plugin.Table{
		Name:        "github_team_member",
		Description: "GitHub members for a given team. GitHub Users are user accounts in GitHub.",
		List: &plugin.ListConfig{
			KeyColumns: []*plugin.KeyColumn{
				{Name: "organization", Require: plugin.Required},
				{Name: "slug", Require: plugin.Required},
				{Name: "role", Require: plugin.Optional},
			},
			Hydrate:           tableGitHubTeamMemberList,
			ShouldIgnoreError: isNotFoundError([]string{"404"}),
		},
		Columns: gitHubTeamMemberColumns(),
	}
}

//// LIST FUNCTION

func tableGitHubTeamMemberList(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
	client := connect(ctx, d)

	opt := &github.TeamListTeamMembersOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		Role:        "all",
	}

	quals := d.KeyColumnQuals
	org := quals["organization"].GetStringValue()
	slug := quals["slug"].GetStringValue()

	// Additional filters
	if quals["role"] != nil {
		opt.Role = quals["role"].GetStringValue()
	}

	type ListPageResponse struct {
		members []*github.User
		resp    *github.Response
	}

	limit := d.QueryContext.Limit
	if limit != nil {
		if *limit < int64(opt.PerPage) {
			opt.PerPage = int(*limit)
		}
	}

	listPage := func(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
		members, resp, err := client.Teams.ListTeamMembersBySlug(ctx, org, slug, opt)
		return ListPageResponse{
			members: members,
			resp:    resp,
		}, err
	}

	for {
		listPageResponse, err := plugin.RetryHydrate(ctx, d, h, listPage, &plugin.RetryConfig{ShouldRetryError: shouldRetryError})

		if err != nil {
			return nil, err
		}

		listResponse := listPageResponse.(ListPageResponse)
		members := listResponse.members
		resp := listResponse.resp

		for _, i := range members {
			if i != nil {
				d.StreamListItem(ctx, i)
			}

			// Context can be cancelled due to manual cancellation or the limit has been hit
			if d.QueryStatus.RowsRemaining(ctx) == 0 {
				return nil, nil
			}
		}

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}

	return nil, nil
}

func tableGitHubTeamMemberGet(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
	org := d.KeyColumnQuals["organization"].GetStringValue()
	slug := d.KeyColumnQuals["slug"].GetStringValue()

	user := h.Item.(*github.User)
	username := *user.Login

	client := connect(ctx, d)

	type GetResponse struct {
		membership *github.Membership
		resp       *github.Response
	}

	getDetails := func(ctx context.Context, d *plugin.QueryData, h *plugin.HydrateData) (interface{}, error) {
		detail, resp, err := client.Teams.GetTeamMembershipBySlug(ctx, org, slug, username)
		return GetResponse{
			membership: detail,
			resp:       resp,
		}, err
	}

	getResponse, err := plugin.RetryHydrate(ctx, d, h, getDetails, &plugin.RetryConfig{ShouldRetryError: shouldRetryError})

	if err != nil {
		return nil, err
	}
	getResp := getResponse.(GetResponse)
	membership := getResp.membership

	return membership, nil
}