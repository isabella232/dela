package encoding

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/any"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/stretchr/testify/require"
)

func TestProtoEncoder_Marshal(t *testing.T) {
	enc := NewProtoEncoder()

	bufferA, err := enc.Marshal(&empty.Empty{})
	require.NoError(t, err)
	bufferB, err := proto.Marshal(&empty.Empty{})
	require.NoError(t, err)
	require.Equal(t, bufferA, bufferB)
}

func TestProtoEncoder_MarshalAny(t *testing.T) {
	enc := NewProtoEncoder()

	anyA, err := enc.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	anyB, err := ptypes.MarshalAny(&empty.Empty{})
	require.NoError(t, err)
	require.Equal(t, anyA, anyB)
}

func TestProtoEncoder_UnmarshalAny(t *testing.T) {
	any, err := ptypes.MarshalAny(&wrappers.UInt64Value{Value: 123})
	require.NoError(t, err)

	enc := NewProtoEncoder()
	msg := &wrappers.UInt64Value{}
	err = enc.UnmarshalAny(any, msg)
	require.NoError(t, err)
	require.Equal(t, uint64(123), msg.GetValue())
}

func TestProtoEncoder_UnmarshalDynamicAny(t *testing.T) {
	packed, err := ptypes.MarshalAny(&wrappers.UInt64Value{Value: 123})
	require.NoError(t, err)

	enc := NewProtoEncoder()
	msg, err := enc.UnmarshalDynamicAny(packed)
	require.NoError(t, err)
	require.IsType(t, (*wrappers.UInt64Value)(nil), msg)

	_, err = enc.UnmarshalDynamicAny(nil)
	require.EqualError(t, err, "message is nil")

	_, err = enc.UnmarshalDynamicAny(&any.Any{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "couldn't unmarshal dynamically: ")
}
