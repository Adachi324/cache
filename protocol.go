package cache

import (
	"encoding/binary"
	"errors"

	"go-eCache/internal/compression"
	"go-eCache/internal/headerproto"
)

const magixPrefixLen = 4
const attrReservedLen = 4
const headerLenReservedLen = 4
const dataLenReservedLen = 4
const hardTimeoutForeverIndicator = 1

// compression magicPrefix and flag
const (
	typeNone   = 0
	typeSnappy = 1
	typeGzip   = 2

	magicPrefix = "_@@_"

	headerFlag = 8 // headerproto flag is the 4th bit in attribute byte
)

var (
	// errEncodingNotMatch is returned if cannot match current bytes protocol
	errEncodingNotMatch = errors.New("cache:encoding: not match bytes protocol")
)

type reservedBytes uint32

func (i reservedBytes) getCompressionType() compression.AlgoType {
	var compressionType compression.AlgoType
	// The lowest 3 bits contain the compression codec used
	var compressionTypeMask uint32 = 0x07
	typeFlag := uint32(i) & compressionTypeMask
	switch typeFlag {
	case typeNone:
		compressionType = compression.None
	case typeSnappy:
		compressionType = compression.Snappy
	case typeGzip:
		compressionType = compression.Gzip
	default:
		compressionType = compression.AlgoType(typeFlag)
	}

	return compressionType
}

func (i reservedBytes) readHeaderFlag() bool {
	return uint32(i)&headerFlag == headerFlag
}

func (i *reservedBytes) writeHeaderFlag() {
	*i |= headerFlag
}

// metaHeader stores meta info for the data bytes
type metaHeader struct {
	SoftTimeoutTs int64
	HardTimeoutTs int64
}

type protocolOption struct {
	softTimeoutTs int64
	hardTimeoutTs int64
}

func newProtocolOption() *protocolOption {
	return &protocolOption{
		softTimeoutTs: 0,
		hardTimeoutTs: 0,
	}
}

// bytesProtocolOption configures bytes protocol behavior
type bytesProtocolOption func(option *protocolOption)

// withSoftTimeoutTs sets timeoutTs
func withSoftTimeoutTs(timeoutTs int64) bytesProtocolOption {
	return func(option *protocolOption) {
		option.softTimeoutTs = timeoutTs
	}
}

// withHardTimeoutTs sets timeoutTs
func withHardTimeoutTs(timeoutTs int64) bytesProtocolOption {
	return func(option *protocolOption) {
		option.hardTimeoutTs = timeoutTs
	}
}

// bytesEncode <data_bytes> into <magic_prefix><attr_bytes><header_len><header_bytes><data_len><original/compressed_data_bytes>
// magic prefix bytes is the identifier of checking whether the bytes has been proceeded by the unified cache lib
func bytesEncode(byt []byte, compressionType compression.AlgoType, opts ...bytesProtocolOption) ([]byte, error) {
	option := newProtocolOption()
	for _, opt := range opts {
		opt(option)
	}

	compressedBytes, err := compression.Compress(byt, compressionType)
	if err != nil {
		return nil, err
	}

	softTimoutTs := option.softTimeoutTs
	hardTimeoutTs := option.hardTimeoutTs
	algoType := int64(compressionType)

	curHeader := &headerproto.Header{
		CompressionType: &algoType,
		SoftTimeoutTs:   &softTimoutTs,
		HardTimeoutTs:   &hardTimeoutTs,
	}

	var headerBytes []byte
	headerBytes, err = curHeader.Marshal()
	if err != nil {
		return nil, err
	}

	var flag reservedBytes
	flag.writeHeaderFlag()

	var finalBytes []byte
	finalBytes = append(finalBytes, []byte(magicPrefix)...)
	finalBytes = appendReservedBytes(finalBytes, flag)
	finalBytes = appendHeaderLenBytes(finalBytes, len(headerBytes))
	finalBytes = append(finalBytes, headerBytes...)
	finalBytes = appendDataLenBytes(finalBytes, len(compressedBytes))
	finalBytes = append(finalBytes, compressedBytes...)

	return finalBytes, nil
}

func appendReservedBytes(byt []byte, flag reservedBytes) []byte {
	b := make([]byte, attrReservedLen)
	binary.LittleEndian.PutUint32(b, uint32(flag))
	byt = append(byt, b...)

	return byt
}

func appendHeaderLenBytes(byt []byte, headerLen int) []byte {
	b := make([]byte, headerLenReservedLen)
	binary.LittleEndian.PutUint32(b, uint32(headerLen))
	byt = append(byt, b...)

	return byt
}

func appendDataLenBytes(byt []byte, dataLen int) []byte {
	b := make([]byte, dataLenReservedLen)
	binary.LittleEndian.PutUint32(b, uint32(dataLen))
	byt = append(byt, b...)

	return byt
}

// bytesDecode <magic_prefix><attr_bytes><header_len><header_bytes><data_len><original/compressed_data_bytes>
// or <magic_prefix><attr_bytes><original/compressed_data_bytes> into headerproto, <original_data_bytes>
func bytesDecode(byt []byte) ([]byte, metaHeader, error) {
	var curHeader metaHeader
	var dataByt []byte
	var compressionType compression.AlgoType

	if isBytesEncoded(byt) {
		rbi := reservedBytes(binary.LittleEndian.Uint32(byt[magixPrefixLen : magixPrefixLen+attrReservedLen]))
		idx := magixPrefixLen + attrReservedLen

		headerFlag := rbi.readHeaderFlag()

		if headerFlag {
			headerLength := rbi.readHeaderLen(byt, &idx)

			receiver := &headerproto.Header{}
			err := receiver.Unmarshal(byt[idx : idx+headerLength])
			if err != nil {
				return nil, metaHeader{}, err
			}

			idx = idx + headerLength
			dataLength := rbi.readDataLen(byt, &idx)

			compressionType = compression.AlgoType(*receiver.CompressionType)
			dataByt = byt[idx : idx+dataLength]

			curHeader.HardTimeoutTs = *receiver.HardTimeoutTs
			curHeader.SoftTimeoutTs = *receiver.SoftTimeoutTs
		} else {
			// the below logic is to provide smooth migration experience
			// for old bytes protocol with bytes layout like `<...magic prefix bytes...><...attribute bytes...><...original/compressed data bytes>`
			compressionType = rbi.getCompressionType()
			dataByt = byt[idx:]
		}

		decompressedBytes, err := compression.Decompress(dataByt, compressionType)
		if err != nil {
			return nil, metaHeader{}, err
		}
		return decompressedBytes, curHeader, nil
	}

	return nil, curHeader, errEncodingNotMatch
}

func (i reservedBytes) readHeaderLen(byt []byte, curIdx *int) int {
	headerLength := int(binary.LittleEndian.Uint32(byt[*curIdx : *curIdx+headerLenReservedLen]))
	*curIdx += headerLenReservedLen

	return headerLength
}

func (i reservedBytes) readDataLen(byt []byte, curIdx *int) int {
	dataLength := int(binary.LittleEndian.Uint32(byt[*curIdx : *curIdx+dataLenReservedLen]))
	*curIdx += dataLenReservedLen

	return dataLength
}

func isBytesEncoded(byt []byte) bool {
	l := len(byt)
	return l >= magixPrefixLen+attrReservedLen && string(byt[:magixPrefixLen]) == magicPrefix
}

type inMemoryItem struct {
	Header metaHeader
	Val    interface{}
}

// nolint:unparam
func inMemoryEncode(val interface{}, opts ...bytesProtocolOption) (inMemoryItem, error) {
	option := newProtocolOption()
	for _, opt := range opts {
		opt(option)
	}

	header := metaHeader{}
	if option.softTimeoutTs != 0 {
		header.SoftTimeoutTs = option.softTimeoutTs
	}
	if option.hardTimeoutTs != 0 {
		header.HardTimeoutTs = option.hardTimeoutTs
	}

	return inMemoryItem{Header: header, Val: val}, nil
}

func inMemoryDecode(value interface{}) (interface{}, metaHeader, error) {
	item, ok := value.(inMemoryItem)
	if !ok {
		return nil, metaHeader{}, cacheErr("unknown_data_from_inMemoryCache")
	}

	return item.Val, item.Header, nil
}
