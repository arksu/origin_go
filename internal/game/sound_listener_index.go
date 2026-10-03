package game

import (
	"math"
	"origin/internal/types"
)

type soundCell struct{ x, y int64 }

type soundListener struct {
	handle       types.Handle
	entityID     types.EntityID
	clientID     uint64
	streamEpoch  uint32
	cell         soundCell
	slot         int
	entries      []soundEntry
	encodedBytes int
}

// Only connected listeners occupy this grid. Its ownership follows the shard
// world lock, so routing never competes with an asynchronous index update.
type soundListenerIndex struct {
	cellSize float64
	cells    map[soundCell][]types.Handle
	members  map[types.Handle]*soundListener
}

func newSoundListenerIndex(cellSize float64) soundListenerIndex {
	return soundListenerIndex{cellSize: cellSize, cells: make(map[soundCell][]types.Handle), members: make(map[types.Handle]*soundListener)}
}

func (index *soundListenerIndex) cellAt(positionX, positionY float64) soundCell {
	return soundCell{int64(math.Floor(positionX / index.cellSize)), int64(math.Floor(positionY / index.cellSize))}
}

func (index *soundListenerIndex) add(handle types.Handle, entityID types.EntityID, clientID uint64, epoch uint32, positionX, positionY float64) {
	index.remove(handle)
	cell := index.cellAt(positionX, positionY)
	index.members[handle] = &soundListener{handle: handle, entityID: entityID, clientID: clientID, streamEpoch: epoch, cell: cell, slot: len(index.cells[cell])}
	index.cells[cell] = append(index.cells[cell], handle)
}

func (index *soundListenerIndex) removeCellMember(listener *soundListener) {
	handles := index.cells[listener.cell]
	last := len(handles) - 1
	if listener.slot != last {
		moved := handles[last]
		handles[listener.slot] = moved
		index.members[moved].slot = listener.slot
	}
	if last == 0 {
		delete(index.cells, listener.cell)
	} else {
		index.cells[listener.cell] = handles[:last]
	}
}

func (index *soundListenerIndex) remove(handle types.Handle) {
	listener, exists := index.members[handle]
	if !exists {
		return
	}
	index.removeCellMember(listener)
	delete(index.members, handle)
}

func (index *soundListenerIndex) move(handle types.Handle, positionX, positionY float64) {
	listener, exists := index.members[handle]
	if !exists {
		return
	}
	cell := index.cellAt(positionX, positionY)
	if cell == listener.cell {
		return
	}
	index.removeCellMember(listener)
	listener.cell, listener.slot = cell, len(index.cells[cell])
	index.cells[cell] = append(index.cells[cell], handle)
}
