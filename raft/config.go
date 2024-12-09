package raft

const (
	StateFollower  = "follower"
	StateCandidate = "candidate"
	StateLeader    = "leader"
)

const HEARTBEAT_TIMEOUT = 300
