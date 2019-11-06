package bot

import (
	"fmt"
	"github.com/bradleyfalzon/ghinstallation"
	"github.com/google/go-github/v28/github"
	"golang.org/x/net/context"
	"net/http"
)

const ApprovedReviewsBeforeReadyToMerge = 2
const LabelWorkInProgress = "work in progress"
const LabelReadyForReview = "ready for review"
const LabelFirstApproval = "first approval"
const LabelReadyToMerge = "ready to merge"

type GithubApp struct {
	client        *github.Client
	webhookSecret []byte
	integrationId int
	privateKey    []byte
}

func New(integrationId int, webhookSecret []byte, privateKey []byte) *GithubApp {
	return &GithubApp{
		client:        github.NewClient(nil),
		webhookSecret: webhookSecret,
		integrationId: integrationId,
		privateKey:    privateKey,
	}
}

func (s *GithubApp) getClientForInstallation(installationId int) (*github.Client, error) {
	itr, err := ghinstallation.New(
		http.DefaultTransport,
		s.integrationId,
		installationId,
		s.privateKey,
	)
	if err != nil {
		return nil, err
	}

	return github.NewClient(&http.Client{Transport: itr}), nil
}

func (s *GithubApp) setupLabelsForAllRepositories(event *github.InstallationEvent) {
	ghi, err := s.getClientForInstallation(int(*event.Installation.ID))
	if err != nil {
		fmt.Printf("Cannot get client for installation %d\n", *event.Installation.ID)

		return
	}

	opt := &github.RepositoryListByOrgOptions{
		Type:        "all",
		ListOptions: github.ListOptions{Page: 1, PerPage: 50},
	}
	for {
		fmt.Printf("Listing repositories page %d\n", opt.Page)
		repositories, resp, err := ghi.Repositories.ListByOrg(context.Background(), *event.Installation.Account.Login, opt)
		if err != nil {
			fmt.Printf("Could not list repositories for %s: %s\n", *event.Installation.Account.Login, err)

			return
		}

		for _, r := range repositories {
			if *r.Archived == true {
				continue
			}

			err := s.createLabels(int(*event.Installation.ID), *event.Installation.Account.Login, *r.Name)
			if err != nil {
				fmt.Printf("Could not create labels for repository %s: %s\n", *r.URL, err)

				return
			}
		}

		if resp.NextPage == 0 {
			break
		}

		opt.Page = resp.NextPage
	}
}

func (s *GithubApp) handlePullRequestCreated(event *github.PullRequestEvent) error {
	ghi, err := s.getClientForInstallation(int(*event.Installation.ID))
	if err != nil {
		return err
	}

	if len(event.PullRequest.Labels) == 0 {
		var label string
		if *event.PullRequest.Draft {
			label = LabelWorkInProgress
		} else {
			label = LabelReadyForReview
		}
		_, _, err = ghi.Issues.AddLabelsToIssue(
			context.Background(),
			*event.Repo.Owner.Login,
			*event.Repo.Name,
			*event.PullRequest.Number,
			[]string{label},
		)
		if err != nil {
			fmt.Printf("Could not add label %s to PR %s\n", label, *event.PullRequest.URL)
			return err
		}
	}

	return nil
}

func (s *GithubApp) handlePullRequestReadyForReview(event *github.PullRequestEvent) error {
	ghi, err := s.getClientForInstallation(int(*event.Installation.ID))
	if err != nil {
		return err
	}

	_, _, err = ghi.Issues.AddLabelsToIssue(
		context.Background(),
		*event.Repo.Owner.Login,
		*event.Repo.Name,
		*event.PullRequest.Number,
		[]string{LabelReadyForReview},
	)
	if err != nil {
		fmt.Printf("Could not add label %s to PR %s\n", LabelReadyForReview, *event.PullRequest.URL)
		return err
	}

	_, err = ghi.Issues.RemoveLabelForIssue(
		context.Background(),
		*event.Repo.Owner.Login,
		*event.Repo.Name,
		*event.PullRequest.Number,
		LabelWorkInProgress,
	)
	if err != nil {
		fmt.Printf("Could not remove label %s to PR %s, it probably doesn't exist\n", LabelWorkInProgress, *event.PullRequest.URL)
	}

	return nil
}

func (s *GithubApp) handlePullRequestReviewed(installationId int, org, repo string, number int, url string) error {
	ghi, err := s.getClientForInstallation(installationId)
	if err != nil {
		return err
	}

	reviews, _, err := ghi.PullRequests.ListReviews(context.Background(), org, repo, number, &github.ListOptions{})
	if err != nil {
		return err
	}

	approvedByUser := make(map[int64]bool)
	approvedReviews := 0
	for _, r := range reviews {
		if *r.State == "APPROVED" && !approvedByUser[*r.User.ID] {
			approvedReviews++

			approvedByUser[*r.User.ID] = true
		}
	}

	fmt.Printf("PR %d received %d reviews\n", number, approvedReviews)

	if approvedReviews == 1 {
		_, _, err = ghi.Issues.AddLabelsToIssue(
			context.Background(),
			org,
			repo,
			number,
			[]string{LabelFirstApproval},
		)
		if err != nil {
			fmt.Printf("Could not add label %s to PR %s\n", LabelFirstApproval, url)
			return err
		}

		_, err = ghi.Issues.RemoveLabelForIssue(
			context.Background(),
			org,
			repo,
			number,
			LabelReadyToMerge,
		)
		if err != nil {
			fmt.Printf("Could not remove label %s to PR %s, it probably doesn't exist\n", LabelReadyToMerge, url)
		}
	} else if approvedReviews >= ApprovedReviewsBeforeReadyToMerge {
		_, _, err = ghi.Issues.AddLabelsToIssue(
			context.Background(),
			org,
			repo,
			number,
			[]string{LabelReadyToMerge},
		)
		if err != nil {
			fmt.Printf("Could not add label %s to PR %s\n", LabelReadyToMerge, url)
			return err
		}

		_, err = ghi.Issues.RemoveLabelForIssue(
			context.Background(),
			org,
			repo,
			number,
			LabelReadyForReview,
		)
		if err != nil {
			fmt.Printf("Could not remove label %s to PR %s, it probably doesn't exist\n", LabelReadyForReview, url)
		}

		_, err = ghi.Issues.RemoveLabelForIssue(
			context.Background(),
			org,
			repo,
			number,
			LabelFirstApproval,
		)
		if err != nil {
			fmt.Printf("Could not remove label %s to PR %s, it probably doesn't exist\n", LabelFirstApproval, url)
		}
	} else {
		_, err = ghi.Issues.RemoveLabelForIssue(
			context.Background(),
			org,
			repo,
			number,
			LabelReadyToMerge,
		)
		if err != nil {
			fmt.Printf("Could not remove label %s to PR %s, it probably doesn't exist\n", LabelReadyToMerge, url)
		}

		_, err = ghi.Issues.RemoveLabelForIssue(
			context.Background(),
			org,
			repo,
			number,
			LabelFirstApproval,
		)
		if err != nil {
			fmt.Printf("Could not remove label %s to PR %s, it probably doesn't exist\n", LabelFirstApproval, url)
		}
	}

	return nil
}

func (s *GithubApp) handleRepositoryCreatedEvent(event *github.RepositoryEvent) error {
	err := s.createLabels(int(*event.Installation.ID), *event.Org.Login, *event.Repo.Name)
	if err != nil {
		return err
	}

	return nil
}

func (s *GithubApp) createLabels(installationId int, owner, repo string) error {
	ghi, err := s.getClientForInstallation(installationId)
	if err != nil {
		return err
	}

	labels, _, err := ghi.Issues.ListLabels(context.Background(), owner, repo, &github.ListOptions{})
	if err != nil {
		return err
	}

	labelsMap := make(map[string]*github.Label, len(labels))
	for _, l := range labels {
		labelsMap[*l.Name] = l

		if owner == "TicketSwap" {
			if *l.Name == "bug" || *l.Name == "duplicate" || *l.Name == "enhancement" || *l.Name == "good first issue" || *l.Name == "help wanted" || *l.Name == "invalid" || *l.Name == "question" || *l.Name == "wontfix" {
				fmt.Printf("Deleting default label %s...\n", *l.Name)

				_, err = ghi.Issues.DeleteLabel(context.Background(), owner, repo, *l.Name)
				if err != nil {
					fmt.Printf("Could not delete label %s: %s\n", *l.Name, err)
					return err
				}
			}
		}
	}

	labelTemplate := map[string]string{
		LabelWorkInProgress: "0052cc",
		LabelFirstApproval:   "bfe5bf",
		LabelReadyForReview: "fef2c0",
		LabelReadyToMerge:   "0e8a16",
	}
	for labelName, labelColor := range labelTemplate {
		label := labelsMap[labelName]

		if label != nil && *label.Color != labelColor {
			fmt.Printf("Label %s exists but has color %s instead of %s, changing...\n", labelName, *label.Color, labelColor)

			label.Color = github.String(labelColor)

			_, _, err = ghi.Issues.EditLabel(context.Background(), owner, repo, labelName, label)
			if err != nil {
				fmt.Printf("Could not edit label %s: %s\n", labelName, err)
				return err
			}
		} else if label == nil {
			fmt.Printf("Label %s does not exist, creating...\n", labelName)
			label = &github.Label{
				Name:  github.String(labelName),
				Color: github.String(labelColor),
			}

			_, _, err = ghi.Issues.CreateLabel(context.Background(), owner, repo, label)
			if err != nil {
				fmt.Printf("Could not create label %s: %s\n", *label.Name, err)
				return err
			}
		}
	}

	return nil
}

func (s *GithubApp) HandlerFunc(w http.ResponseWriter, r *http.Request) {
	payload, err := github.ValidatePayload(r, s.webhookSecret)
	if err != nil {
		fmt.Printf("Invalid signature\n")

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Invalid signature"))

		return
	}

	event, err := github.ParseWebHook(github.WebHookType(r), payload)
	if err != nil {
		fmt.Printf("Cannot parse webhook payload\n")

		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte("Cannot parse webhook payload"))

		return
	}

	switch event := event.(type) {
	case *github.InstallationEvent:
		fmt.Printf("Installation %s for %s\n", *event.Action, *event.Installation.Account.Login)

		if *event.Action == "created" {
			go s.setupLabelsForAllRepositories(event)
		}
	case *github.PullRequestReviewEvent:
		if *event.Review.State == "approved" && *event.Action == "submitted" {
			s.handlePullRequestReviewed(
				int(*event.Installation.ID),
				*event.Organization.Login,
				*event.Repo.Name,
				*event.PullRequest.Number,
				*event.PullRequest.URL,
			)
		}
	case *github.PullRequestEvent:
		if *event.Action == "opened" {
			s.handlePullRequestCreated(event)
		} else if *event.Action == "ready_for_review" {
			s.handlePullRequestReadyForReview(event)
		} else if *event.Action == "labeled" {
			//s.applyPullRequestLabels(
			//	int(*event.Installation.ID),
			//	*event.Repo.Owner.Login,
			//	*event.Repo.Name,
			//	*event.PullRequest.Number,
			//	*event.PullRequest.URL,
			//)
		}
	case *github.RepositoryEvent:
		if *event.Action == "created" {
			err = s.handleRepositoryCreatedEvent(event)
			if err != nil {
				fmt.Printf("Cannot handle repository created event: %s\n", err)

				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte("Cannot handle repository created event"))

				return
			}
		}
	default:
		fmt.Printf("Skipping event %+v\n", event)
	}

	w.WriteHeader(http.StatusOK)
}
