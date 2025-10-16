package signer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestConsensusLockIntegration tests that consensus lock doesn't interfere with normal signing operations
func TestConsensusLockIntegration(t *testing.T) {
	// Create a simple sign state with no existing lock
	signState := &SignState{
		Height:        100,
		Round:         5,
		Step:          stepPrecommit,
		ConsensusLock: ConsensusLock{}, // No lock initially
	}

	// Test that we can sign a PROPOSAL without issues
	proposalBytes := []byte("proposal_block_hash_123456789012345678901234567890")
	err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPropose}, proposalBytes)
	require.NoError(t, err, "Should allow PROPOSAL signing when no lock exists")

	// Test that we can sign a PREVOTE without issues
	prevoteBytes := []byte("prevote_block_hash_123456789012345678901234567890")
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPrevote}, prevoteBytes)
	require.NoError(t, err, "Should allow PREVOTE signing when no lock exists")

	// Test that we can sign a PRECOMMIT without issues
	precommitBytes := []byte("precommit_block_hash_123456789012345678901234567890")
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPrecommit}, precommitBytes)
	require.NoError(t, err, "Should allow PRECOMMIT signing when no lock exists")

	// Test that we can sign at different heights without issues
	err = signState.ValidateConsensusLock(HRSKey{Height: 101, Round: 1, Step: stepPropose}, proposalBytes)
	require.NoError(t, err, "Should allow signing at different height")

	// Test that we can sign at different rounds within same height without issues
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPropose}, proposalBytes)
	require.NoError(t, err, "Should allow signing at different round within same height")
}

// TestConsensusLockPerformance tests that consensus lock validation is fast
func TestConsensusLockPerformance(t *testing.T) {
	signState := &SignState{
		Height:        100,
		Round:         5,
		Step:          stepPrecommit,
		ConsensusLock: ConsensusLock{}, // No lock initially
	}

	blockBytes := []byte("block_hash_123456789012345678901234567890")

	// Test that validation is fast (should complete in < 1ms)
	start := time.Now()
	for i := 0; i < 1000; i++ {
		err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 5, Step: stepPropose}, blockBytes)
		require.NoError(t, err)
	}
	duration := time.Since(start)

	// Should complete 1000 validations in well under 1 second
	require.Less(t, duration, time.Second, "Consensus lock validation should be fast")
}
