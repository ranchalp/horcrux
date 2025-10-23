package signer

import (
	"encoding/hex"
	"testing"
)

func TestValidateConsensusLockWithForceUnlock(t *testing.T) {
	// Create a sign state with a lock
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrevote,
		ConsensusLock: ConsensusLock{
			Height: 100,
			Round:  2, // Locked in round 2
			Value:  []byte("locked_block_hash_123456789012345678901234567890")[:32],
		},
	}

	// Test case 1: PREVOTE for locked value - should allow
	lockedBlockBytes := createTestSignBytes(signState.ConsensusLock.Value, stepPrevote)
	hrs := HRSKey{Height: 100, Round: 5, Step: stepPrevote}

	err := signState.ValidateConsensusLockWithConsensusState(hrs, lockedBlockBytes)
	if err != nil {
		t.Errorf("Should allow PREVOTE for locked value: %v", err)
	}

	// Test case 2: PREVOTE for different value without consensus justification - should block
	differentBlock := []byte("different_block_hash_123456789012345678901234567890")[:32]
	differentBlockBytes := createTestSignBytes(differentBlock, stepPrevote)

	err = signState.ValidateConsensusLockWithConsensusState(hrs, differentBlockBytes)
	if err == nil {
		t.Error("Should block PREVOTE for different value without consensus justification")
	}

	// Test case 3: PREVOTE for different value with consensus justification - should allow
	// Add a prevote quorum for the different block to simulate consensus justification
	signState.AddPrevoteQuorum(100, differentBlock, 3) // Add quorum from round 3
	err = signState.ValidateConsensusLockWithConsensusState(hrs, differentBlockBytes)
	if err != nil {
		t.Errorf("Should allow PREVOTE for different value with consensus justification: %v", err)
	}

	// Test case 4: PROPOSAL for different value with consensus justification - should still block (only PREVOTE allows unlock)
	err = signState.ValidateConsensusLockWithConsensusState(
		HRSKey{Height: 100, Round: 5, Step: stepPropose},
		createTestSignBytes(differentBlock, stepPropose))
	if err == nil {
		t.Error("Should block PROPOSAL for different value even with consensus justification")
	}
}

func TestOptimalRoundTracking(t *testing.T) {
	// Test that we track the oldest round (> 0) for optimal bypass
	signState := &SignState{
		Height: 100,
		ConsensusLock: ConsensusLock{
			Height: 100,
			Round:  2,
			Value:  []byte("locked_block_hash_123456789012345678901234567890")[:32],
		},
	}

	// Add multiple quorums for the same value with different rounds
	signState.AddPrevoteQuorum(100, []byte("test_block_hash_123456789012345678901234567890")[:32], 5) // Round 5
	signState.AddPrevoteQuorum(100, []byte("test_block_hash_123456789012345678901234567890")[:32], 3) // Round 3 (older)
	signState.AddPrevoteQuorum(100, []byte("test_block_hash_123456789012345678901234567890")[:32], 7) // Round 7 (newer)

	// Should be able to unlock with round 3 (oldest > 0)
	hrs := HRSKey{Height: 100, Round: 4, Step: stepPrevote}
	blockBytes := createTestSignBytes([]byte("test_block_hash_123456789012345678901234567890")[:32], stepPrevote)

	err := signState.ValidateConsensusLockWithConsensusState(hrs, blockBytes)
	if err != nil {
		t.Errorf("Should allow PREVOTE with optimal round tracking: %v", err)
	}

	// Verify we kept the oldest round (3) not the newest (7)
	heightMap := signState.PrevoteQuorums["100"]
	if heightMap == nil {
		t.Fatal("Prevote quorums should exist")
	}

	valueKey := hex.EncodeToString([]byte("test_block_hash_123456789012345678901234567890")[:32])
	round, exists := heightMap[valueKey]
	if !exists {
		t.Error("Should have found the quorum")
	} else if round != 3 {
		t.Errorf("Expected round 3 (oldest), got round %d", round)
	}
}

func TestBackwardsCompatibility(t *testing.T) {
	// Test that old ValidateConsensusLock still works
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrevote,
		ConsensusLock: ConsensusLock{
			Height: 100,
			Round:  2,
			Value:  []byte("locked_block_hash_123456789012345678901234567890")[:32],
		},
	}

	// Old method should work (defaults to not allowing unlock for backwards compatibility)
	differentBlock := []byte("different_block_hash_123456789012345678901234567890")[:32]
	differentBlockBytes := createTestSignBytes(differentBlock, stepPrevote)
	hrs := HRSKey{Height: 100, Round: 5, Step: stepPrevote}

	err := signState.ValidateConsensusLock(hrs, differentBlockBytes)
	if err == nil {
		t.Error("Backwards compatibility should block different value by default")
	}
}

func TestPrevoteQuorumCleanup(t *testing.T) {
	// Test that prevote quorums are cleaned up when moving to new heights
	signState := &SignState{
		Height:         100,
		PrevoteQuorums: make(map[string]map[string]int64),
	}

	// Add prevote quorums for different heights
	signState.AddPrevoteQuorum(95, []byte("block_hash_95"), 1)
	signState.AddPrevoteQuorum(98, []byte("block_hash_98"), 1)
	signState.AddPrevoteQuorum(99, []byte("block_hash_99"), 2)
	signState.AddPrevoteQuorum(100, []byte("block_hash_100"), 3)

	// Verify all heights exist (no cleanup yet)
	if len(signState.PrevoteQuorums) != 4 {
		t.Errorf("Expected 4 heights, got %d", len(signState.PrevoteQuorums))
	}

	// Test cleanup by manually calling it (simulating height change)
	signState.cleanupOldPrevoteQuorums(101)

	// Verify cleanup - height 95 should be removed (101-3=98, so 95 < 98), others remain
	expectedHeights := 3 // 98, 99, 100 should remain
	if len(signState.PrevoteQuorums) != expectedHeights {
		t.Errorf("Expected %d heights after cleanup, got %d", expectedHeights, len(signState.PrevoteQuorums))
	}

	// Verify specific heights
	if _, exists := signState.PrevoteQuorums["95"]; exists {
		t.Error("Height 95 should have been cleaned up")
	}
	if _, exists := signState.PrevoteQuorums["98"]; !exists {
		t.Error("Height 98 should still exist (within cache)")
	}
	if _, exists := signState.PrevoteQuorums["99"]; !exists {
		t.Error("Height 99 should still exist")
	}
	if _, exists := signState.PrevoteQuorums["100"]; !exists {
		t.Error("Height 100 should still exist")
	}
}
