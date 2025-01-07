package raft

// exported function to initiate sending heartbeats for testing
func (node *RaftNode) InitiateHeartbeatProcess() {
	go node.heartbeatBroadcast()
}

// exported function to handle role transitions for testing
func (node *RaftNode) TransitionState(newState string) {
	node.transitionState(newState)
}
