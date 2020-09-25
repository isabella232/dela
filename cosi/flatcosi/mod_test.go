package flatcosi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/cosi"
	"go.dedis.ch/dela/crypto"
	"go.dedis.ch/dela/crypto/bls"
	"go.dedis.ch/dela/internal/testing/fake"
)

func TestFlat_GetSigner(t *testing.T) {
	flat := NewFlat(nil, fake.NewAggregateSigner())
	require.NotNil(t, flat.GetSigner())
}

func TestFlat_GetPublicKeyFactory(t *testing.T) {
	flat := NewFlat(nil, fake.NewAggregateSigner())
	require.NotNil(t, flat.GetPublicKeyFactory())
}

func TestFlat_GetSignatureFactory(t *testing.T) {
	flat := NewFlat(nil, fake.NewAggregateSigner())
	require.NotNil(t, flat.GetSignatureFactory())
}

func TestFlat_GetVerifierFactory(t *testing.T) {
	flat := NewFlat(nil, fake.NewAggregateSigner())
	require.NotNil(t, flat.GetVerifierFactory())
}

func TestFlat_Listen(t *testing.T) {
	flat := NewFlat(fake.Mino{}, bls.NewSigner())

	a, err := flat.Listen(fakeReactor{})
	require.NoError(t, err)
	actor := a.(flatActor)
	require.NotNil(t, actor.signer)
	require.NotNil(t, actor.rpc)

	flat.mino = fake.NewBadMino()
	_, err = flat.Listen(fakeReactor{})
	require.EqualError(t, err, fake.Err("couldn't make the rpc"))
}

func TestActor_Sign(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		signer:  fake.NewAggregateSigner(),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	rpc.SendResponse(nil, cosi.SignatureResponse{Signature: fake.Signature{}})
	rpc.SendResponse(nil, cosi.SignatureResponse{Signature: fake.Signature{}})
	rpc.Done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig, err := actor.Sign(ctx, message, ca)
	require.NoError(t, err)
	require.NotNil(t, sig)

	actor.rpc = fake.NewBadRPC()
	_, err = actor.Sign(ctx, message, ca)
	require.EqualError(t, err, fake.Err("call aborted"))

	actor.rpc = rpc
	actor.signer = fake.NewSignerWithVerifierFactory(fake.NewBadVerifierFactory())
	_, err = actor.Sign(ctx, message, ca)
	require.EqualError(t, err, fake.Err("couldn't make verifier"))

	actor.signer = fake.NewAggregateSigner()
	actor.reactor = fakeReactor{err: fake.GetError()}
	_, err = actor.Sign(ctx, message, ca)
	require.EqualError(t, err, fake.Err("couldn't react to message"))
}

func TestActor_SignWrongSignature(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		signer:  fake.NewSignerWithVerifierFactory(fake.NewVerifierFactory(fake.NewBadVerifier())),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	rpc.SendResponse(nil, cosi.SignatureResponse{Signature: fake.Signature{}})
	rpc.Done()

	ctx := context.Background()

	_, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, fake.Err("couldn't verify the aggregation"))
}

func TestActor_RPCError_Sign(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		signer:  ca.GetSigner(0).(crypto.AggregateSigner),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	rpc.Done()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, "signature is nil")
	require.Nil(t, sig)
}

func TestActor_Context_Sign(t *testing.T) {
	message := fake.Message{}
	ca := fake.NewAuthority(1, fake.NewSigner)
	rpc := fake.NewRPC()

	actor := flatActor{
		signer:  ca.GetSigner(0).(crypto.AggregateSigner),
		rpc:     rpc,
		reactor: fakeReactor{},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	rpc.SendResponseWithError(nil, fake.GetError())
	rpc.Done()

	sig, err := actor.Sign(ctx, message, ca)
	require.EqualError(t, err, fake.Err("one request has failed"))
	require.Nil(t, sig)
}

func TestActor_SignProcessError(t *testing.T) {
	ca := fake.NewAuthority(1, fake.NewSigner)

	rpc := fake.NewRPC()
	actor := flatActor{
		signer:  ca.GetSigner(0).(crypto.AggregateSigner),
		reactor: fakeReactor{},
		rpc:     rpc,
	}

	rpc.SendResponse(nil, fake.Message{})
	rpc.Done()
	_, err := actor.Sign(context.Background(), fake.Message{}, ca)
	require.EqualError(t, err,
		"couldn't process response: invalid response type 'fake.Message'")

	actor.signer = fake.NewBadSigner()
	_, err = actor.processResponse(cosi.SignatureResponse{}, fake.Signature{})
	require.EqualError(t, err, fake.Err("couldn't aggregate"))
}
