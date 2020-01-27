package helpers

import (
	"github.com/google/go-github/v28/github"
	"golang.org/x/net/context"
)

func GetPullRequestWithTitle(client *github.Client, ctx context.Context, repoOwner string, repoName string, title string) (*github.PullRequest, error) {
	pullRequestList, _, err := client.PullRequests.List(ctx, repoOwner, repoName, nil)
	if err != nil {
		return nil, err

	}

	var matchingPR *github.PullRequest
	for _, pullRequest := range pullRequestList {
		if *pullRequest.Title == title {
			matchingPR = pullRequest
		}
	}

	return matchingPR, nil
}
