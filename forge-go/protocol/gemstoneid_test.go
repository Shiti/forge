package protocol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIDGenerator(t *testing.T) {
	generator, err := NewGemstoneGenerator(1)
	require.NoError(t, err)

	id1, err := generator.Generate(PriorityNormal)
	require.NoError(t, err)

	id2, err := generator.Generate(PriorityNormal)
	require.NoError(t, err)

	assert.NotEqual(t, id1, id2)
}

func TestIDToIntAndBack(t *testing.T) {
	generator, err := NewGemstoneGenerator(1)
	require.NoError(t, err)

	id1, err := generator.Generate(PriorityUrgent)
	require.NoError(t, err)

	idInt := id1.ToInt()
	id2, err := ParseGemstoneID(idInt)
	require.NoError(t, err)

	assert.Equal(t, id1.Priority, id2.Priority)
	assert.Equal(t, id1.Timestamp, id2.Timestamp)
	assert.Equal(t, id1.MachineID, id2.MachineID)
	assert.Equal(t, id1.SequenceNumber, id2.SequenceNumber)
}

func TestIDToStringAndBack(t *testing.T) {
	generator, err := NewGemstoneGenerator(1)
	require.NoError(t, err)

	id1, err := generator.Generate(PriorityNormal)
	require.NoError(t, err)

	idStr := id1.ToString()
	id2, err := ParseGemstoneIDString(idStr)
	require.NoError(t, err)

	assert.Equal(t, id1, id2)
}

func TestClockMovedBackwardsError(t *testing.T) {
	generator, err := NewGemstoneGenerator(1)
	require.NoError(t, err)

	generator.lastTimestamp = time.Now().UnixMilli() + 10000

	_, err = generator.Generate(PriorityNormal)
	assert.ErrorIs(t, err, ErrClockMovedBackwards)
}

func TestMachineIDBoundary(t *testing.T) {
	_, err := NewGemstoneGenerator(256)
	assert.ErrorIs(t, err, ErrInvalidMachineID)

	_, err = NewGemstoneGenerator(-1)
	assert.ErrorIs(t, err, ErrInvalidMachineID)
}

func TestSequenceMaxInMillisecond(t *testing.T) {
	generator, err := NewGemstoneGenerator(1)
	require.NoError(t, err)

	// Since Go is fast, we can test generating 4096 IDs quickly
	id1, _ := generator.Generate(PriorityNormal)
	for i := 0; i < 4095; i++ {
		generator.Generate(PriorityNormal)
	}
	id2, _ := generator.Generate(PriorityNormal)
	for i := 0; i < 4095; i++ {
		generator.Generate(PriorityNormal)
	}
	id3, _ := generator.Generate(PriorityNormal)

	assert.True(t, id2.Timestamp > id1.Timestamp)
	assert.True(t, id3.Timestamp > id2.Timestamp)
}

func TestInvalidPriority(t *testing.T) {
	generator, err := NewGemstoneGenerator(1)
	require.NoError(t, err)

	_, err = generator.Generate(Priority(8))
	assert.ErrorIs(t, err, ErrInvalidPriority)

	_, err = generator.Generate(Priority(-1))
	assert.ErrorIs(t, err, ErrInvalidPriority)
}

func TestIDOrderingByPriority(t *testing.T) {
	generator, _ := NewGemstoneGenerator(1)
	id1, _ := generator.Generate(PriorityLow)
	id2, _ := generator.Generate(PriorityHigh)
	// Lower enum value numerical means HIGHER priority (e.g. High = 2, Low = 5).
	// Therefore id2 (High/2) is less than id1 (Low/5) numerically when encoded or compared natively.
	// Wait, Python's: (self.priority, ...), where priority is the numeric int.
	// Python HIGH = 2, LOW = 5.
	// So Python says id_high (2) < id_low (5)
	// That means id_high < id_low.
	assert.True(t, Compare(id2, id1) < 0)
}

func TestContractGeneration(t *testing.T) {
	// A simple test generating many IDs to ensure it doesn't crash or panic.
	generator, _ := NewGemstoneGenerator(42)
	for i := 0; i < 1000; i++ {
		id, err := generator.Generate(PriorityNormal)
		require.NoError(t, err)
		assert.Equal(t, 42, id.MachineID)
		assert.Equal(t, PriorityNormal, id.Priority)

		intId := id.ToInt()
		parsed, err := ParseGemstoneID(intId)
		require.NoError(t, err)
		assert.Equal(t, id, parsed)
	}
}
