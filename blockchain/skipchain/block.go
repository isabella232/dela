package skipchain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	fmt "fmt"

	proto "github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"go.dedis.ch/fabric/blockchain"
	"go.dedis.ch/fabric/consensus"
	"go.dedis.ch/fabric/crypto"
	"go.dedis.ch/fabric/encoding"
	"golang.org/x/xerrors"
)

const (
	// DefaultMaximumHeight is the default value when creating a genesis block
	// for the maximum height of each block.
	DefaultMaximumHeight = 32

	// DefaultBaseHeight is the default value when creating a genesis block
	// for the base height of each block.
	DefaultBaseHeight = 4
)

var (
	protoenc encoding.ProtoMarshaler = encoding.NewProtoEncoder()
)

// SkipBlock is a representation of the data held by a block. It contains the
// information to build a skipchain.
type SkipBlock struct {
	hash          []byte
	Index         uint64
	Conodes       Conodes
	Height        uint32
	BaseHeight    uint32
	MaximumHeight uint32
	GenesisID     blockchain.BlockID
	DataHash      []byte
	BackLinks     []blockchain.BlockID
	Payload       proto.Message
}

// GetID returns the unique identifier for this block.
func (b SkipBlock) GetID() blockchain.BlockID {
	id := blockchain.BlockID{}
	copy(id[:], b.hash)
	return id
}

// GetHash returns the hash of the block.
func (b SkipBlock) GetHash() []byte {
	return b.hash
}

// Pack returns the protobuf message for a block.
func (b SkipBlock) Pack() (proto.Message, error) {
	payloadAny, err := protoenc.MarshalAny(b.Payload)
	if err != nil {
		return nil, encoding.NewAnyEncodingError(b.Payload, err)
	}

	backLinks := make([][]byte, len(b.BackLinks))
	for i, bl := range b.BackLinks {
		backLinks[i] = bl.Bytes()
	}

	conodes, err := b.Conodes.ToProto()
	if err != nil {
		return nil, err
	}

	blockproto := &BlockProto{
		Index:         b.Index,
		Height:        b.Height,
		BaseHeight:    b.BaseHeight,
		MaximumHeight: b.MaximumHeight,
		GenesisID:     b.GenesisID.Bytes(),
		DataHash:      b.DataHash,
		Backlinks:     backLinks,
		Payload:       payloadAny,
		Conodes:       conodes,
	}

	return blockproto, nil
}

func (b SkipBlock) String() string {
	return fmt.Sprintf("Block[%v]", b.GetID())
}

func (b SkipBlock) computeHash() ([]byte, error) {
	h := sha256.New()

	buffer := make([]byte, 20)
	binary.LittleEndian.PutUint64(buffer[0:8], b.Index)
	binary.LittleEndian.PutUint32(buffer[8:12], b.Height)
	binary.LittleEndian.PutUint32(buffer[12:16], b.BaseHeight)
	binary.LittleEndian.PutUint32(buffer[16:20], b.MaximumHeight)
	h.Write(buffer)
	if b.Conodes != nil {
		b.Conodes.WriteTo(h)
	}
	h.Write(b.GenesisID.Bytes())
	h.Write(b.DataHash)

	for _, bl := range b.BackLinks {
		h.Write(bl.Bytes())
	}

	return h.Sum(nil), nil
}

// VerifiableBlock is a block combined with a consensus chain that can be
// verified from the genesis.
type VerifiableBlock struct {
	SkipBlock
	Chain consensus.Chain
}

// Verify makes sure the integrity of the chain is valid.
func (vb VerifiableBlock) Verify(v crypto.Verifier) error {
	return nil
}

// Pack returns the protobuf message for a verifiable block.
func (vb VerifiableBlock) Pack() (proto.Message, error) {
	block, err := vb.SkipBlock.Pack()
	if err != nil {
		return nil, err
	}

	packed := &VerifiableBlockProto{
		Block: block.(*BlockProto),
		Chain: nil, // TODO
	}

	return packed, nil
}

// blockFactory is responsible for the instantiation of the block and related
// data structures like the forward links and the proves.
type blockFactory struct {
	genesis      *SkipBlock
	verifier     crypto.Verifier
	chainFactory consensus.ChainFactory
}

func newBlockFactory(v crypto.Verifier, cf consensus.ChainFactory) *blockFactory {
	return &blockFactory{
		verifier:     v,
		chainFactory: cf,
	}
}

func (f *blockFactory) createGenesis(conodes []Conode, data proto.Message) (SkipBlock, error) {
	h := sha256.New()
	if data == nil {
		data = &empty.Empty{}
	}

	buffer, err := protoenc.Marshal(data)
	if err != nil {
		return SkipBlock{}, encoding.NewEncodingError("data", err)
	}

	h.Write(buffer)

	// TODO: crypto module for randomness
	randomBackLink := blockchain.BlockID{}
	rand.Read(randomBackLink[:])

	genesis := SkipBlock{
		Index:         0,
		Conodes:       conodes,
		Height:        1,
		BaseHeight:    DefaultBaseHeight,
		MaximumHeight: DefaultMaximumHeight,
		GenesisID:     blockchain.BlockID{},
		DataHash:      h.Sum(nil),
		BackLinks:     []blockchain.BlockID{randomBackLink},
		Payload:       data,
	}

	genesis.hash, err = genesis.computeHash()
	if err != nil {
		return genesis, xerrors.Errorf("couldn't hash the genesis block: %v", err)
	}

	f.genesis = &genesis

	return genesis, nil
}

func (f *blockFactory) fromPrevious(prev SkipBlock, data proto.Message) (SkipBlock, error) {
	databuf, err := protoenc.Marshal(data)
	if err != nil {
		return SkipBlock{}, encoding.NewEncodingError("data", err)
	}

	h := sha256.New()
	h.Write(databuf)

	backlink := blockchain.BlockID{}
	copy(backlink[:], prev.hash)

	block := SkipBlock{
		Index:         prev.Index + 1,
		Conodes:       prev.Conodes,
		Height:        prev.Height,
		BaseHeight:    prev.BaseHeight,
		MaximumHeight: prev.MaximumHeight,
		GenesisID:     prev.GenesisID,
		DataHash:      h.Sum(nil),
		BackLinks:     []blockchain.BlockID{backlink},
		Payload:       data,
	}

	hash, err := block.computeHash()
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't hash the block: %v", err)
	}

	block.hash = hash

	return block, nil
}

func (f *blockFactory) fromBlock(src proto.Message) (SkipBlock, error) {
	in := src.(*BlockProto)

	var payload ptypes.DynamicAny
	err := protoenc.UnmarshalAny(in.GetPayload(), &payload)
	if err != nil {
		return SkipBlock{}, encoding.NewAnyDecodingError(&payload, err)
	}

	backLinks := make([]blockchain.BlockID, len(in.GetBacklinks()))
	for i, packed := range in.GetBacklinks() {
		backLinks[i] = blockchain.NewBlockID(packed)
	}

	conodes, err := NewConodesFromProto(f.verifier, in.GetConodes())
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't create the conodes: %v", err)
	}

	block := SkipBlock{
		Index:         in.GetIndex(),
		Conodes:       conodes,
		Height:        in.GetHeight(),
		BaseHeight:    in.GetBaseHeight(),
		MaximumHeight: in.GetMaximumHeight(),
		GenesisID:     blockchain.NewBlockID(in.GetGenesisID()),
		DataHash:      in.GetDataHash(),
		BackLinks:     backLinks,
		Payload:       payload.Message,
	}

	hash, err := block.computeHash()
	if err != nil {
		return SkipBlock{}, xerrors.Errorf("couldn't hash the block: %v", err)
	}

	block.hash = hash

	return block, nil
}

func (f *blockFactory) FromVerifiable(src proto.Message) (blockchain.Block, error) {
	in, ok := src.(*VerifiableBlockProto)
	if !ok {
		return nil, xerrors.New("unknown type")
	}

	if f.genesis == nil {
		return nil, xerrors.New("genesis block not initialized")
	}

	block, err := f.fromBlock(in.GetBlock())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the block: %v", err)
	}

	chain, err := f.chainFactory.FromProto(in.GetChain())
	if err != nil {
		return nil, xerrors.Errorf("couldn't decode the chain: %v", err)
	}

	err = chain.Verify(f.verifier, f.genesis.Conodes.GetPublicKeys())
	if err != nil {
		return nil, err
	}

	return block, nil
}

// PayloadValidator is the interface to implement to validate the payload
// when a new block is proposed.
type PayloadValidator interface {
	Validate(payload proto.Message) error
}

type blockValidator struct{}

func (v blockValidator) Validate(previous []byte, pb proto.Message) (consensus.Proposal, error) {
	return SkipBlock{}, nil
}
