package game

import (
	"fmt"
	"math"
	_const "origin/internal/const"
	"origin/internal/network"
	netproto "origin/internal/network/proto"
	"origin/internal/timeutil"

	"google.golang.org/protobuf/proto"
)

const directionalMovementSupported = true

func marshalServerConstants(tickRate int) ([]byte, error) {
	if tickRate <= 0 || uint64(tickRate) > math.MaxUint32 {
		return nil, fmt.Errorf("invalid server tick rate: %d", tickRate)
	}
	return proto.Marshal(&netproto.ServerMessage{
		Payload: &netproto.ServerMessage_ServerConstants{
			ServerConstants: &netproto.S2C_ServerConstants{
				CoordPerTile:                 _const.CoordPerTile,
				ChunkSize:                    _const.ChunkSize,
				TickRate:                     uint32(tickRate),
				DirectionalMovementSupported: directionalMovementSupported,
				RealSecondsPerGameDay:        timeutil.RealSecondsPerGameDay,
				HoursPerDay:                  timeutil.HoursPerDay,
				DaysPerMonth:                 timeutil.DaysPerMonth,
				MonthsPerYear:                timeutil.MonthsPerYear,
			},
		},
	})
}

// The connection's sequential read loop cannot process its next Ping until both
// critical packets are queued. Spawn may only start after this returns true.
func (g *Game) sendAuthenticatedBootstrap(c *network.Client, sequence uint32) bool {
	select {
	case <-c.Done():
		return false
	default:
	}
	if len(g.serverConstantsData) == 0 {
		g.logger.Error("Server constants are not initialized")
		c.Close()
		return false
	}
	if !g.sendAuthResult(c, sequence, true, "") {
		return false
	}
	if !c.SendCritical(g.serverConstantsData) {
		c.Close()
		return false
	}
	select {
	case <-c.Done():
		return false
	default:
	}
	return true
}
