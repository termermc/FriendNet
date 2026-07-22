package common

// RepoUrl is the software's repository URL.
const RepoUrl = "https://github.com/termermc/friendnet"

// IssuesUrl is the software's issue tracker URL.
const IssuesUrl = RepoUrl + "/issues"

// NewIssueUrl is the URL for creating a new issue.
const NewIssueUrl = IssuesUrl + "/new"

// StunResTimeoutSeconds is the number of seconds a STUN resolution attempt will live before being timed out
const StunResTimeoutSeconds = 3

// HolePunchTickMs is the number of milliseconds between sending garbage to during NAT holepunching.
const HolePunchTickMs = 100

// HolePunchGarbageLength is the packet size for garbage sent during a hole punch
const HolePunchGarbageLength = 256
