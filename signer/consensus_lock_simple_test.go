package signer

import (
	"strings"
	"testing"
)

func TestConsensusLockBasic(t *testing.T) {
	// Create a new sign state with no lock
	signState := &SignState{
		Height:        100,
		Round:         5,
		Step:          stepPrevote,
		ConsensusLock: ConsensusLock{}, // No lock initially
	}

	// Test that no lock means no validation error
	blockBytes := []byte("some_block_data_123456789012345678901234567890")
	err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPrevote}, blockBytes)
	if err != nil {
		t.Errorf("Expected no error when no lock exists, got: %v", err)
	}

	// Test updating lock on PRECOMMIT
	blockHash := []byte("new_block_hash_123456789012345678901234567890") // 32 bytes
	signState.ConsensusLock = updateConsensusLock(
		signState.ConsensusLock, HRSKey{Height: 100, Round: 5, Step: stepPrecommit}, blockHash)

	if !signState.ConsensusLock.IsLocked() {
		t.Error("Expected consensus lock to be set after PRECOMMIT")
	}

	if signState.ConsensusLock.Height != 100 {
		t.Errorf("Expected lock height 100, got %d", signState.ConsensusLock.Height)
	}

	if signState.ConsensusLock.Round != 5 {
		t.Errorf("Expected lock round 5, got %d", signState.ConsensusLock.Round)
	}

	// Test that lock is preserved correctly
	expectedValue := blockHash[:32] // First 32 bytes
	if string(signState.ConsensusLock.Value) != string(expectedValue) {
		t.Errorf("Expected lock value %s, got %s", string(expectedValue), string(signState.ConsensusLock.Value))
	}
}

func TestConsensusLockValidationWithLock(t *testing.T) {
	// Create a sign state with a lock
	lockedValue := []byte("locked_block_hash_123456789012345678901234567890")
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrecommit,
		ConsensusLock: ConsensusLock{
			Height:    100,
			Round:     5,
			Value:     lockedValue[:32], // First 32 bytes
			ValueType: "block",
		},
	}

	// Test 1: Try to sign the same block at same round (should succeed)
	sameBlockBytes := []byte("locked_block_hash_123456789012345678901234567890_other_data")
	err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPrevote}, sameBlockBytes)
	if err != nil {
		t.Errorf("Expected no error when signing same block at same round, got: %v", err)
	}

	// Test 2: Try to sign a different block at the same round (should fail for PROPOSAL/PREVOTE)
	differentBlockBytes := []byte("different_block_hash_123456789012345678901234567890_other_data")
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPrevote}, differentBlockBytes)
	if err == nil {
		t.Error("Expected error when signing different block at same round, got nil")
	}

	// Test 3: Try to sign the locked value at a later round (should succeed)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote}, sameBlockBytes)
	if err != nil {
		t.Errorf("Expected no error when signing locked value at later round, got: %v", err)
	}

	// Test 4: Try to sign a different value at a later round (should fail for PROPOSAL/PREVOTE)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote}, differentBlockBytes)
	if err == nil {
		t.Error("Expected error when signing different value at later round, got nil")
	}

	// Test 5: Try to sign PRECOMMIT at later round (should succeed - releases lock)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrecommit}, differentBlockBytes)
	if err != nil {
		t.Errorf("Expected no error when signing PRECOMMIT at later round, got: %v", err)
	}

	// Test 6: Try to sign at a different height (should succeed - lock is released)
	err = signState.ValidateConsensusLock(HRSKey{Height: 101, Round: 5, Step: stepPrevote}, differentBlockBytes)
	if err != nil {
		t.Errorf("Expected no error when signing at different height, got: %v", err)
	}
}

func TestConsensusLockErrorTypes(t *testing.T) {
	// Create a sign state with a lock
	lockedValue := []byte("locked_block_hash_123456789012345678901234567890")
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrecommit,
		ConsensusLock: ConsensusLock{
			Height:    100,
			Round:     5,
			Value:     lockedValue[:32], // First 32 bytes
			ValueType: "block",
		},
	}

	// Test that we get a ConsensusLockViolationError for conflicting values
	differentBlockBytes := []byte("different_block_hash_123456789012345678901234567890")
	err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote}, differentBlockBytes)
	if err == nil {
		t.Error("Expected consensus lock violation error, got nil")
	}

	// Test that it's specifically a ConsensusLockViolationError
	if !IsConsensusLockViolationError(err) {
		t.Errorf("Expected ConsensusLockViolationError, got %T", err)
	}

	// Test that the error message contains the expected information
	expectedMsg := "consensus lock violation: locked on value"
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("Expected error message to contain '%s', got: %s", expectedMsg, err.Error())
	}

	// Test that we don't get an error for the same value
	sameBlockBytes := []byte("locked_block_hash_123456789012345678901234567890")
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote}, sameBlockBytes)
	if err != nil {
		t.Errorf("Expected no error for same value, got: %v", err)
	}

	// Test that we don't get an error for different height
	err = signState.ValidateConsensusLock(HRSKey{Height: 101, Round: 5, Step: stepPrevote}, differentBlockBytes)
	if err != nil {
		t.Errorf("Expected no error for different height, got: %v", err)
	}
}

func TestConsensusLockClearing(t *testing.T) {
	// Create a sign state with a lock
	lockedValue := []byte("locked_block_hash_123456789012345678901234567890")
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrecommit,
		ConsensusLock: ConsensusLock{
			Height:    100,
			Round:     5,
			Value:     lockedValue[:32], // First 32 bytes
			ValueType: "block",
		},
	}

	// Test 1: Clear lock when moving to different height
	signState.ClearConsensusLock(HRSKey{Height: 101, Round: 5, Step: stepPrevote})
	if signState.ConsensusLock.IsLocked() {
		t.Error("Expected lock to be cleared when moving to different height")
	}

	// Reset the lock
	signState.ConsensusLock = ConsensusLock{
		Height:    100,
		Round:     5,
		Value:     lockedValue[:32],
		ValueType: "block",
	}

	// Test 2: Don't clear lock when moving to higher round (locks persist for all future rounds)
	signState.ClearConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote})
	if !signState.ConsensusLock.IsLocked() {
		t.Error("Expected lock to remain when moving to higher round (locks persist for all future rounds)")
	}

	// Reset the lock
	signState.ConsensusLock = ConsensusLock{
		Height:    100,
		Round:     5,
		Value:     lockedValue[:32],
		ValueType: "block",
	}

	// Test 3: Don't clear lock when moving to same or lower round
	signState.ClearConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPrevote})
	if !signState.ConsensusLock.IsLocked() {
		t.Error("Expected lock to remain when moving to same round")
	}

	signState.ClearConsensusLock(HRSKey{Height: 100, Round: 4, Step: stepPrevote})
	if !signState.ConsensusLock.IsLocked() {
		t.Error("Expected lock to remain when moving to lower round")
	}
}
