package common

import "time"

// RepoUrl is the software's repository URL.
const RepoUrl = "https://github.com/termermc/friendnet"

// IssuesUrl is the software's issue tracker URL.
const IssuesUrl = RepoUrl + "/issues"

// NewIssueUrl is the URL for creating a new issue.
const NewIssueUrl = IssuesUrl + "/new"

// StunResTimeout is the time a STUN resolution attempt will live before being timed out
const StunResTimeout = 3 * time.Second

const HolePunchAttempts = 3

const HolePunchConnBackoffMaxTimeout = 3 * time.Second
