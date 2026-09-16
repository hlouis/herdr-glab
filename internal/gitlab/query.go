package gitlab

const mrFragment = `
fragment mr on MergeRequest {
  iid title webUrl draft updatedAt
  sourceBranch targetBranch diffHeadSha
  detailedMergeStatus
  approved approvalsLeft
  resolvableDiscussionsCount resolvedDiscussionsCount userNotesCount
  author { username }
  project { fullPath }
  sourceProject { fullPath }
  headPipeline { status }
  reviewers { nodes { username mergeRequestInteraction { reviewState approved } } }
}
`

// mineQuery fetches the three "related to me" lists. Lists that have no more
// pages are skipped through @include so pagination only refetches what is left.
const mineQuery = `
query($withAuthored: Boolean!, $withReview: Boolean!, $withAssigned: Boolean!,
      $authoredAfter: String, $reviewAfter: String, $assignedAfter: String) {
  currentUser {
    username
    authoredMergeRequests(state: opened, first: 50, after: $authoredAfter) @include(if: $withAuthored) {
      pageInfo { hasNextPage endCursor } nodes { ...mr }
    }
    reviewRequestedMergeRequests(state: opened, first: 50, after: $reviewAfter) @include(if: $withReview) {
      pageInfo { hasNextPage endCursor } nodes { ...mr }
    }
    assignedMergeRequests(state: opened, first: 50, after: $assignedAfter) @include(if: $withAssigned) {
      pageInfo { hasNextPage endCursor } nodes { ...mr }
    }
  }
}
` + mrFragment

type mrNode struct {
	IID                        string `json:"iid"`
	Title                      string `json:"title"`
	WebURL                     string `json:"webUrl"`
	Draft                      bool   `json:"draft"`
	UpdatedAt                  string `json:"updatedAt"`
	SourceBranch               string `json:"sourceBranch"`
	TargetBranch               string `json:"targetBranch"`
	DiffHeadSha                string `json:"diffHeadSha"`
	DetailedMergeStatus        string `json:"detailedMergeStatus"`
	Approved                   bool   `json:"approved"`
	ApprovalsLeft              *int   `json:"approvalsLeft"`
	ResolvableDiscussionsCount int    `json:"resolvableDiscussionsCount"`
	ResolvedDiscussionsCount   int    `json:"resolvedDiscussionsCount"`
	UserNotesCount             int    `json:"userNotesCount"`
	Author                     *struct {
		Username string `json:"username"`
	} `json:"author"`
	Project struct {
		FullPath string `json:"fullPath"`
	} `json:"project"`
	SourceProject *struct {
		FullPath string `json:"fullPath"`
	} `json:"sourceProject"`
	HeadPipeline *struct {
		Status string `json:"status"`
	} `json:"headPipeline"`
	Reviewers struct {
		Nodes []struct {
			Username                string `json:"username"`
			MergeRequestInteraction *struct {
				ReviewState string `json:"reviewState"`
				Approved    bool   `json:"approved"`
			} `json:"mergeRequestInteraction"`
		} `json:"nodes"`
	} `json:"reviewers"`
}

type mrPage struct {
	PageInfo struct {
		HasNextPage bool   `json:"hasNextPage"`
		EndCursor   string `json:"endCursor"`
	} `json:"pageInfo"`
	Nodes []mrNode `json:"nodes"`
}

type mineData struct {
	CurrentUser *struct {
		Username                     string  `json:"username"`
		AuthoredMergeRequests        *mrPage `json:"authoredMergeRequests"`
		ReviewRequestedMergeRequests *mrPage `json:"reviewRequestedMergeRequests"`
		AssignedMergeRequests        *mrPage `json:"assignedMergeRequests"`
	} `json:"currentUser"`
}
