package minogrpc

import (
	context "context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.dedis.ch/dela/internal/testing/fake"
	"go.dedis.ch/dela/mino"
	"go.dedis.ch/dela/mino/minogrpc/routing"
	"go.dedis.ch/dela/serde"
	"go.dedis.ch/dela/serde/json"
	"golang.org/x/xerrors"
	"google.golang.org/grpc"
)

func TestRPC_Call(t *testing.T) {
	rpc := &RPC{
		factory: fake.MessageFactory{},
		overlay: overlay{
			connFactory: fakeConnFactory{},
			serializer:  json.NewSerializer(),
		},
	}

	addrs := []mino.Address{address{"A"}, address{"B"}}

	msgs, errs := rpc.Call(context.Background(), fake.Message{}, mino.NewAddresses(addrs...))
	select {
	case err := <-errs:
		t.Fatal(err)
	case msg := <-msgs:
		require.NotNil(t, msg)
	}
}

func TestRPC_Stream(t *testing.T) {
	addrs := []mino.Address{address{"A"}, address{"B"}}

	rpc := &RPC{
		overlay: overlay{
			me:             addrs[0],
			routingFactory: routing.NewTreeRoutingFactory(1, AddressFactory{}),
			connFactory:    fakeConnFactory{},
			serializer:     json.NewSerializer(),
		},
	}

	out, in, err := rpc.Stream(context.Background(), mino.NewAddresses(addrs...))
	require.NoError(t, err)

	out.Send(fake.Message{}, newRootAddress())
	in.Recv(context.Background())

	rpc.overlay.routingFactory = badRtingFactory{}
	_, _, err = rpc.Stream(context.Background(), mino.NewAddresses())
	require.EqualError(t, err, "couldn't generate routing: oops")

	rpc.overlay.routingFactory = routing.NewTreeRoutingFactory(1, AddressFactory{})
	rpc.overlay.connFactory = fakeConnFactory{err: xerrors.New("oops")}
	_, _, err = rpc.Stream(context.Background(), mino.NewAddresses())
	require.EqualError(t, err,
		"couldn't setup relay: couldn't open connection: oops")
}

// -----------------------------------------------------------------------------
// Utility functions

type fakeClientStream struct {
	grpc.ClientStream
	init *Envelope
	ch   chan *Envelope
	err  error
}

func (str *fakeClientStream) SendMsg(m interface{}) error {
	if str.err != nil {
		return str.err
	}

	if str.init == nil {
		str.init = m.(*Envelope)
		return nil
	}

	str.ch <- m.(*Envelope)
	return nil
}

func (str *fakeClientStream) RecvMsg(m interface{}) error {
	msg, more := <-str.ch
	if !more {
		return io.EOF
	}

	*(m.(*Envelope)) = *msg
	return nil
}

type fakeConnection struct {
	grpc.ClientConnInterface
	resp      interface{}
	err       error
	errStream error
}

func (conn fakeConnection) Invoke(ctx context.Context, m string, arg interface{},
	resp interface{}, opts ...grpc.CallOption) error {

	switch msg := resp.(type) {
	case *Message:
		*msg = Message{
			Payload: []byte(`{}`),
		}
	case *JoinResponse:
		*msg = conn.resp.(JoinResponse)
	default:
	}

	return conn.err
}

func (conn fakeConnection) NewStream(ctx context.Context, desc *grpc.StreamDesc,
	m string, opts ...grpc.CallOption) (grpc.ClientStream, error) {

	ch := make(chan *Envelope, 1)

	go func() {
		<-ctx.Done()
		close(ch)
	}()

	return &fakeClientStream{ch: ch, err: conn.errStream}, conn.err
}

type fakeConnFactory struct {
	ConnectionFactory
	resp      interface{}
	err       error
	errConn   error
	errStream error
}

func (f fakeConnFactory) FromAddress(mino.Address) (grpc.ClientConnInterface, error) {
	conn := fakeConnection{
		resp:      f.resp,
		err:       f.errConn,
		errStream: f.errStream,
	}

	return conn, f.err
}

type badRtingFactory struct {
	routing.Factory
}

func (f badRtingFactory) VisitJSON(serde.FactoryInput) (serde.Message, error) {
	return nil, xerrors.New("oops")
}

func (f badRtingFactory) FromIterator(mino.Address, mino.AddressIterator) (routing.Routing, error) {
	return nil, xerrors.New("oops")
}
