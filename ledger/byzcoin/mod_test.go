package byzcoin

import (
	"context"
	"testing"
	"time"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/stretchr/testify/require"
	"go.dedis.ch/fabric/crypto"
	internal "go.dedis.ch/fabric/internal/testing"
	"go.dedis.ch/fabric/ledger/consumer"
	"go.dedis.ch/fabric/ledger/consumer/smartcontract"
	"go.dedis.ch/fabric/mino"
	"go.dedis.ch/fabric/mino/minoch"
)

func TestMessages(t *testing.T) {
	messages := []proto.Message{
		&BlockPayload{},
	}

	for _, m := range messages {
		internal.CoverProtoMessage(t, m)
	}
}

func TestLedger_Basic(t *testing.T) {
	manager := minoch.NewManager()

	m, err := minoch.NewMinoch(manager, "A")
	require.NoError(t, err)

	ledger := NewLedger(m, makeConsumer())
	roster := roster{members: []*Ledger{ledger}}

	actor, err := ledger.Listen(roster)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trs := ledger.Watch(ctx)

	tx, err := smartcontract.NewTransactionFactory().
		NewSpawn(simpleContractName, &empty.Empty{})
	require.NoError(t, err)

	err = actor.AddTransaction(tx)
	require.NoError(t, err)

	select {
	case res := <-trs:
		require.NotNil(t, res)
		require.Equal(t, tx.GetID(), res.GetTransactionID())
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timeout")
	}

}

// -----------------------------------------------------------------------------
// Utility functions

type simpleContract struct{}

const simpleContractName = "simpleContract"

func (c simpleContract) Spawn(ctx smartcontract.SpawnContext) (proto.Message, error) {
	return &empty.Empty{}, nil
}

func (c simpleContract) Invoke(ctx smartcontract.InvokeContext) (proto.Message, error) {
	return &empty.Empty{}, nil
}

func makeConsumer() consumer.Consumer {
	c := smartcontract.NewConsumer()
	c.Register(simpleContractName, simpleContract{})
	return c
}

type addressIterator struct {
	index   int
	members []*Ledger
}

func (i *addressIterator) HasNext() bool {
	return i.index+1 < len(i.members)
}

func (i *addressIterator) GetNext() mino.Address {
	if i.HasNext() {
		i.index++
		return i.members[i.index].addr
	}
	return nil
}

type publicKeyIterator struct {
	index   int
	members []*Ledger
}

func (i *publicKeyIterator) HasNext() bool {
	return i.index+1 < len(i.members)
}

func (i *publicKeyIterator) GetNext() crypto.PublicKey {
	if i.HasNext() {
		i.index++
		return i.members[i.index].signer.GetPublicKey()
	}
	return nil
}

type roster struct {
	members []*Ledger
}

func (r roster) Len() int {
	return len(r.members)
}

func (r roster) Take(...mino.FilterUpdater) mino.Players {
	return roster{members: r.members}
}

func (r roster) AddressIterator() mino.AddressIterator {
	return &addressIterator{index: -1, members: r.members}
}

func (r roster) PublicKeyIterator() crypto.PublicKeyIterator {
	return &publicKeyIterator{index: -1, members: r.members}
}
