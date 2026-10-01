package game

import "origin/internal/types"

func (s *Shard) validDirectionalSession(playerID types.EntityID, clientID uint64, epoch uint32) bool {
	s.ClientsMu.RLock()
	defer s.ClientsMu.RUnlock()
	client := s.Clients[playerID]
	return client != nil && client.ID == clientID && client.InWorld.Load() && epoch != 0 && client.StreamEpoch.Load() == epoch
}
