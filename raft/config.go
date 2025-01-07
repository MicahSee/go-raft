package raft

const (
	StateFollower  = "follower"
	StateCandidate = "candidate"
	StateLeader    = "leader"
)

const HEARTBEAT_TIMEOUT = 500
const MIN_ELECTION_TIMEOUT = 300
const MAX_ELECTION_TIMEOUT = 450
