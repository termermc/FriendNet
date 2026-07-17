package common

import "os"

// RepoUrl is the software's repository URL.
const RepoUrl = "https://github.com/termermc/friendnet"

// IssuesUrl is the software's issue tracker URL.
const IssuesUrl = RepoUrl + "/issues"

// NewIssueUrl is the URL for creating a new issue.
const NewIssueUrl = IssuesUrl + "/new"

// IsDebugMode is true if the software is running in debug mode.
var IsDebugMode = os.Getenv("DEBUG") != "" && os.Getenv("DEBUG") != "0"
