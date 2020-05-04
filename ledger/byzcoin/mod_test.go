package byzcoin

import (
	"context"
	fmt "fmt"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/crypto/bls"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/internal/testing/fake"
	"go.dedis.ch/fabric/ledger"
	"go.dedis.ch/fabric/ledger/arc/darc"
	"go.dedis.ch/fabric/ledger/byzcoin/roster"
	"go.dedis.ch/fabric/ledger/transactions/basic"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&BlockPayload{},
		&GenesisPayload{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

// This test checks the basic behaviour of a Byzcoin ledger. The module should
// do the following steps without errors:
// 1. Run n nodes and start to listen for requests
// 2. Setup the ledger on the leader (as we use a leader-based view change)
// 3. Send transactions and accept them.
func TestLedger_Basic(t *testing.T) {
	ledgers, actors, ca := makeLedger(t, 20)
	defer func() {
		for _, actor := range actors {
			require.NoError(t, actor.Close())
		}
	}()

	require.NoError(t, actors[0].Setup(ca))

	for _, actor := range actors {
		err := <-actor.HasStarted()
		require.NoError(t, err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	txs := ledgers[2].Watch(ctx)

	signer := bls.NewSigner()
	txFactory := basic.NewTransactionFactory(signer, nil)

	// Execute a roster change tx by removing one of the participants.
	tx, err := txFactory.New(roster.NewClientTask([]uint32{15}))
	require.NoError(t, err)

	err = actors[1].AddTransaction(tx)
	require.NoError(t, err)

	select {
	case res := <-txs:
		require.NotNil(t, res)
		require.Equal(t, tx.GetID(), res.GetTransactionID())
	case <-time.After(1 * time.Second):
		t.Fatal("timeout 1")
	}

	roster, err := ledgers[5].(*Ledger).governance.GetAuthority(1)
	require.NoError(t, err)
	require.Equal(t, 19, roster.Len())

	// Try to create a DARC.
	access := makeDarc(t, signer)
	tx, err = txFactory.New(darc.NewCreate(access))
	require.NoError(t, err)

	err = actors[1].AddTransaction(tx)
	require.NoError(t, err)

	select {
	case res := <-txs:
		require.NotNil(t, res)
		require.Equal(t, tx.GetID(), res.GetTransactionID())
	case <-time.After(1 * time.Second):
		t.Fatal("timeout 2")
	}

	value, err := ledgers[2].GetValue(tx.GetID())
	require.NoError(t, err)
	require.IsType(t, (*darc.AccessProto)(nil), value)

	// Then update it.
	tx, err = txFactory.New(darc.NewUpdate(tx.GetID(), access))
	require.NoError(t, err)

	err = actors[0].AddTransaction(tx)
	require.NoError(t, err)

	select {
	case res := <-txs:
		require.NotNil(t, res)
		require.Equal(t, tx.GetID(), res.GetTransactionID())
	case <-time.After(1 * time.Second):
		t.Fatal("timeout 3")
	}
}

// -----------------------------------------------------------------------------
// Utility functions

func makeLedger(t *testing.T, n int) ([]ledger.Ledger, []ledger.Actor, crypto.CollectiveAuthority) {
	manager := minoch.NewManager()

	minos := make([]mino.Mino, n)
	for i := 0; i < n; i++ {
		m, err := minoch.NewMinoch(manager, fmt.Sprintf("node%d", i))
		require.NoError(t, err)

		minos[i] = m
	}

	ca := fake.NewAuthorityFromMino(bls.NewSigner, minos...)
	ledgers := make([]ledger.Ledger, n)
	actors := make([]ledger.Actor, n)
	for i, m := range minos {
		ledger := NewLedger(m, ca.GetSigner(i))
		ledgers[i] = ledger

		actor, err := ledger.Listen()
		require.NoError(t, err)

		actors[i] = actor
	}

	return ledgers, actors, ca
}

func makeDarc(t *testing.T, signer crypto.Signer) darc.Access {
	access := darc.NewAccess()
	access, err := access.Evolve(darc.UpdateAccessRule, signer.GetPublicKey())
	require.NoError(t, err)

	return access
}
