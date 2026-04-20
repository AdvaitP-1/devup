package scheduler

import (
	"sort"

	"devup/internal/api"
	"devup/internal/version"
)

// Pick selects the best node from peers using a least-loaded-first strategy.
// localNodeID identifies the local agent so ties can prefer local execution
// (avoiding upload overhead). Returns nil if no eligible node exists.
func Pick(peers []api.PeerInfo, localNodeID string) *api.PeerInfo {
	var eligible []api.PeerInfo
	for _, p := range peers {
		if p.SlotsFree <= 0 {
			continue
		}
		if !majorMatch(p.Version) {
			continue
		}
		eligible = append(eligible, p)
	}
	if len(eligible) == 0 {
		return nil
	}

	sort.Slice(eligible, func(i, j int) bool {
		si := score(eligible[i])
		sj := score(eligible[j])
		if si != sj {
			return si > sj
		}
		// Tie-break: prefer local to avoid upload overhead
		return eligible[i].NodeID == localNodeID
	})

	return &eligible[0]
}

// Rank returns all eligible peers sorted by score descending (best first).
// Used by the fail-forward retry loop.
func Rank(peers []api.PeerInfo, localNodeID string) []api.PeerInfo {
	var eligible []api.PeerInfo
	for _, p := range peers {
		if p.SlotsFree <= 0 {
			continue
		}
		if !majorMatch(p.Version) {
			continue
		}
		eligible = append(eligible, p)
	}

	sort.Slice(eligible, func(i, j int) bool {
		si := score(eligible[i])
		sj := score(eligible[j])
		if si != sj {
			return si > sj
		}
		return eligible[i].NodeID == localNodeID
	})

	return eligible
}

func score(p api.PeerInfo) int {
	return (p.SlotsFree * 100) + p.MemFreeMB
}

func majorMatch(peerVersion string) bool {
	if peerVersion == "" {
		return false
	}
	// Parse major from "X.Y.Z"
	dot := 0
	for dot < len(peerVersion) && peerVersion[dot] != '.' {
		dot++
	}
	major := 0
	for i := 0; i < dot; i++ {
		if peerVersion[i] < '0' || peerVersion[i] > '9' {
			return false
		}
		major = major*10 + int(peerVersion[i]-'0')
	}
	return major == version.Major
}
