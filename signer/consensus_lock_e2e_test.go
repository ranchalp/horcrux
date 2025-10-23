package signer

import (
	"testing"
	"time"

	"github.com/cometbft/cometbft/libs/protoio"
	cometproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"
)

// createTestSignBytes creates proper Tendermint sign bytes for testing
func createTestSignBytesE2E(blockHash []byte, step int8) []byte {
	switch step {
	case stepPropose:
		// Create a CanonicalProposal
		proposal := &cometproto.CanonicalProposal{
			Type:   cometproto.ProposalType,
			Height: 100,
			Round:  5,
			BlockID: &cometproto.CanonicalBlockID{
				Hash: blockHash,
			},
		}
		signBytes, _ := protoio.MarshalDelimited(proposal)
		return signBytes

	case stepPrevote, stepPrecommit:
		// Create a CanonicalVote
		vote := &cometproto.CanonicalVote{
			Type:   cometproto.SignedMsgType(step),
			Height: 100,
			Round:  5,
			BlockID: &cometproto.CanonicalBlockID{
				Hash: blockHash,
			},
		}
		signBytes, _ := protoio.MarshalDelimited(vote)
		return signBytes

	default:
		return nil
	}
}

// TestConsensusLockE2E tests the consensus lock functionality end-to-end
// This test simulates a real scenario where a validator tries to sign conflicting blocks
func TestConsensusLockE2E(t *testing.T) {
	// Create a sign state that simulates a validator that has already signed a PRECOMMIT
	// for a specific block at height 100, round 5
	lockedBlockHash := []byte("locked_block_hash_123456789012345678901234567890")[:32] // Ensure exactly 32 bytes
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrecommit,
		ConsensusLock: ConsensusLock{
			Height: 100,
			Round:  5,
			Value:  lockedBlockHash,
		},
	}

	// Test 1: Validator tries to sign a PROPOSAL for the same block in a later round
	// This should be allowed (same value)
	sameBlockProposal := createTestSignBytesE2E(lockedBlockHash, stepPropose)
	err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPropose}, sameBlockProposal)
	require.NoError(t, err, "Should allow PROPOSAL for same block in later round")

	// Test 2: Validator tries to sign a PREVOTE for the same block in a later round
	// This should be allowed (same value)
	sameBlockPrevote := createTestSignBytesE2E(lockedBlockHash, stepPrevote)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote}, sameBlockPrevote)
	require.NoError(t, err, "Should allow PREVOTE for same block in later round")

	// Test 3: Validator tries to sign a PROPOSAL for a different block in a later round
	// This should be blocked (different value)
	differentBlockHash := []byte("different_block_hash_123456789012345678901234567890")[:32]
	differentBlockProposal := createTestSignBytesE2E(differentBlockHash, stepPropose)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPropose}, differentBlockProposal)
	require.Error(t, err, "Should block PROPOSAL for different block in later round")
	require.True(t, IsConsensusLockViolationError(err), "Should be a consensus lock violation error")

	// Test 4: Validator tries to sign a PREVOTE for a different block in a later round
	// This should be blocked (different value)
	differentBlockPrevote := createTestSignBytesE2E(differentBlockHash, stepPrevote)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote}, differentBlockPrevote)
	require.Error(t, err, "Should block PREVOTE for different block in later round")
	require.True(t, IsConsensusLockViolationError(err), "Should be a consensus lock violation error")

	// Test 5: Validator tries to sign a PRECOMMIT for a different block in a later round
	// This should be allowed (PRECOMMIT releases the lock)
	differentBlockPrecommit := createTestSignBytesE2E(differentBlockHash, stepPrecommit)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrecommit}, differentBlockPrecommit)
	require.NoError(t, err, "Should allow PRECOMMIT for different block in later round (releases lock)")

	// Test 6: After signing a PRECOMMIT for a different block, the lock should be updated
	// Simulate the lock update
	newLock := nextConsensusLock(signState.ConsensusLock, HRSKey{Height: 100, Round: 6, Step: stepPrecommit}, differentBlockPrecommit)
	require.True(t, newLock.IsLocked(), "New lock should be active")
	require.Equal(t, int64(100), newLock.Height, "Lock should be at height 100")
	require.Equal(t, int64(6), newLock.Round, "Lock should be at round 6")
	require.Equal(t, differentBlockHash, newLock.Value, "Lock should be on the new block")

	// Test 7: Validator tries to sign for a different height
	// This should be allowed (locks are height-specific)
	differentHeightBytes := createTestSignBytesE2E(differentBlockHash, stepPropose)
	err = signState.ValidateConsensusLock(HRSKey{Height: 101, Round: 1, Step: stepPropose}, differentHeightBytes)
	require.NoError(t, err, "Should allow signing for different height")

	// Test 8: Validator tries to sign for the same height but earlier round
	// This should be allowed (locks only apply to later rounds)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 4, Step: stepPropose}, differentHeightBytes)
	require.NoError(t, err, "Should allow signing for earlier round")
}

// TestConsensusLockRealWorldScenario tests a realistic scenario
func TestConsensusLockRealWorldScenario(t *testing.T) {
	// Scenario: Validator is locked on block A at height 100, round 5
	// Network times out and moves to round 6
	// New block B is proposed in round 6
	// Validator should be prevented from signing block B until it signs a PRECOMMIT

	lockedBlockA := []byte("block_A_hash_123456789012345678901234567890")[:32]
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrecommit,
		ConsensusLock: ConsensusLock{
			Height: 100,
			Round:  5,
			Value:  lockedBlockA,
		},
	}

	// Round 6: New block B is proposed
	blockB := []byte("block_B_hash_123456789012345678901234567890")[:32]

	// Validator should NOT be able to sign PROPOSAL for block B
	blockBProposal := createTestSignBytesE2E(blockB, stepPropose)
	err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPropose}, blockBProposal)
	require.Error(t, err, "Should block PROPOSAL for block B")
	require.True(t, IsConsensusLockViolationError(err), "Should be a consensus lock violation")

	// Validator should NOT be able to sign PREVOTE for block B
	blockBPrevote := createTestSignBytesE2E(blockB, stepPrevote)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrevote}, blockBPrevote)
	require.Error(t, err, "Should block PREVOTE for block B")
	require.True(t, IsConsensusLockViolationError(err), "Should be a consensus lock violation")

	// Validator should be able to sign PRECOMMIT for block B (this releases the lock)
	blockBPrecommit := createTestSignBytesE2E(blockB, stepPrecommit)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPrecommit}, blockBPrecommit)
	require.NoError(t, err, "Should allow PRECOMMIT for block B (releases lock)")

	// After signing PRECOMMIT for block B, validator should be locked on block B
	newLock := nextConsensusLock(signState.ConsensusLock, HRSKey{Height: 100, Round: 6, Step: stepPrecommit}, blockBPrecommit)
	require.True(t, newLock.IsLocked(), "Should be locked on block B")
	require.Equal(t, blockB, newLock.Value, "Lock should be on block B")
	require.Equal(t, int64(6), newLock.Round, "Lock should be at round 6")

	// Update the signState with the new lock
	signState.ConsensusLock = newLock

	// Now validator should NOT be able to sign for block A in round 7
	blockAProposal := createTestSignBytesE2E(lockedBlockA, stepPropose)
	err = signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 7, Step: stepPropose}, blockAProposal)
	require.Error(t, err, "Should block PROPOSAL for block A in round 7")
	require.True(t, IsConsensusLockViolationError(err), "Should be a consensus lock violation")
}

// TestConsensusLockPerformanceE2E tests that consensus lock validation is fast enough for production use
func TestConsensusLockPerformanceE2E(t *testing.T) {
	signState := &SignState{
		Height: 100,
		Round:  5,
		Step:   stepPrecommit,
		ConsensusLock: ConsensusLock{
			Height: 100,
			Round:  5,
			Value:  []byte("locked_block_hash_123456789012345678901234567890")[:32],
		},
	}

	blockHash := []byte("different_block_hash_123456789012345678901234567890")[:32]
	blockBytes := createTestSignBytesE2E(blockHash, stepPropose)

	// Test that validation is fast (should complete in < 1ms per operation)
	start := time.Now()
	for i := 0; i < 10000; i++ {
		err := signState.ValidateConsensusLock(HRSKey{Height: 100, Round: 6, Step: stepPropose}, blockBytes)
		require.Error(t, err) // Should always fail due to lock violation
	}
	duration := time.Since(start)

	// Should complete 10,000 validations in well under 1 second
	require.Less(t, duration, time.Second, "Consensus lock validation should be very fast")

	avgTimePerOp := duration / 10000
	require.Less(t, avgTimePerOp, 100*time.Microsecond, "Average time per operation should be < 100μs")
}
