package protocol

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"
)

var (
	// EPOCH is 2023-01-01 00:00:00 UTC in milliseconds
	EPOCH int64 = time.Date(2023, time.January, 1, 0, 0, 0, 0, time.UTC).UnixMilli()

	ErrClockMovedBackwards = errors.New("clock moved backwards")
	ErrInvalidPriority     = errors.New("invalid priority value")
	ErrInvalidMachineID    = errors.New("invalid machine_id value")
	ErrInvalidSequence     = errors.New("invalid sequence_number value")
	ErrInvalidIDString     = errors.New("invalid ID string format")
)

const (
	MachineIDBitmask = 0xFF
	SequenceBitmask  = 0xFFF
	PriorityBitmask  = 0x7

	PriorityShift  = 61
	TimestampShift = 22
	MachineIDShift = 12
)

type Priority int

const (
	PriorityUrgent      Priority = 0
	PriorityImportant   Priority = 1
	PriorityHigh        Priority = 2
	PriorityAboveNormal Priority = 3
	PriorityNormal      Priority = 4
	PriorityLow         Priority = 5
	PriorityVeryLow     Priority = 6
	PriorityLowest      Priority = 7
)

type GemstoneID struct {
	Priority       Priority
	Timestamp      int64
	MachineID      int
	SequenceNumber int
}

func NewGemstoneID(priority Priority, timestamp int64, machineID int, sequenceNumber int) (GemstoneID, error) {
	if priority < 0 || priority > 7 {
		return GemstoneID{}, ErrInvalidPriority
	}
	if machineID < 0 || machineID > 255 {
		return GemstoneID{}, ErrInvalidMachineID
	}
	if sequenceNumber < 0 || sequenceNumber > 4095 {
		return GemstoneID{}, ErrInvalidSequence
	}
	return GemstoneID{
		Priority:       priority,
		Timestamp:      timestamp,
		MachineID:      machineID,
		SequenceNumber: sequenceNumber,
	}, nil
}

func (id GemstoneID) ToInt() uint64 {
	p := uint64(id.Priority&PriorityBitmask) << PriorityShift
	t := uint64(id.Timestamp-EPOCH) << TimestampShift
	m := uint64(id.MachineID&MachineIDBitmask) << MachineIDShift
	s := uint64(id.SequenceNumber & SequenceBitmask)

	return p | t | m | s
}

func (id GemstoneID) ToString() string {
	return strconv.FormatUint(id.ToInt(), 10)
}

func ParseGemstoneID(id uint64) (GemstoneID, error) {
	priority := Priority((id >> PriorityShift) & PriorityBitmask)
	timestamp := int64((id>>TimestampShift)&((1<<(PriorityShift-TimestampShift))-1)) + EPOCH
	machineID := int((id >> MachineIDShift) & ((1 << (TimestampShift - MachineIDShift)) - 1))
	sequenceNumber := int(id & SequenceBitmask)

	return NewGemstoneID(priority, timestamp, machineID, sequenceNumber)
}

func ParseGemstoneIDString(idStr string) (GemstoneID, error) {
	id, err := strconv.ParseUint(idStr, 10, 64)
	if err != nil {
		return GemstoneID{}, fmt.Errorf("%w: %v", ErrInvalidIDString, err)
	}
	return ParseGemstoneID(id)
}

type GemstoneGenerator struct {
	mu             sync.Mutex
	machineID      int
	sequenceNumber int
	lastTimestamp  int64
}

func NewGemstoneGenerator(machineID int) (*GemstoneGenerator, error) {
	if machineID < 0 || machineID > 255 {
		return nil, ErrInvalidMachineID
	}
	return &GemstoneGenerator{
		machineID:     machineID,
		lastTimestamp: -1,
	}, nil
}

func (g *GemstoneGenerator) Generate(priority Priority) (GemstoneID, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if priority < 0 || priority > 7 {
		return GemstoneID{}, ErrInvalidPriority
	}

	timestamp := time.Now().UnixMilli()
	if timestamp < g.lastTimestamp {
		return GemstoneID{}, ErrClockMovedBackwards
	}

	if timestamp == g.lastTimestamp {
		g.sequenceNumber = (g.sequenceNumber + 1) & SequenceBitmask
		if g.sequenceNumber == 0 {
			for timestamp <= g.lastTimestamp {
				time.Sleep(time.Millisecond)
				timestamp = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequenceNumber = 0
	}

	g.lastTimestamp = timestamp

	return NewGemstoneID(priority, timestamp, g.machineID, g.sequenceNumber)
}

func Compare(a, b GemstoneID) int {
	if a.Priority != b.Priority {
		return int(a.Priority) - int(b.Priority)
	}
	if a.Timestamp != b.Timestamp {
		return int(a.Timestamp - b.Timestamp)
	}
	if a.MachineID != b.MachineID {
		return a.MachineID - b.MachineID
	}
	if a.SequenceNumber != b.SequenceNumber {
		return a.SequenceNumber - b.SequenceNumber
	}
	return 0
}
