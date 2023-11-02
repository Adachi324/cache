package compression

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io/ioutil"
	"sync"

	"github.com/golang/snappy"
)

// AlgoType is the compression algorithm type
type AlgoType int

// Generally speaking `Snappy` performs best in terms of CPU cost with a bit sacrifice in compression ratio
// If your application has much more Read than Write and storage is not a big concern, please consider using `Snappy`
const (
	Snappy AlgoType = 0 // compress/decompress using Snappy
	Gzip   AlgoType = 1 // compress/decompress using Gzip with default compression level
	None   AlgoType = 2 // Not apply any compression algorithm
)

const (
	// DefaultMinLenForCompression  will apply when the compression config's MinLenForCompression is 0.
	DefaultMinLenForCompression = 512
)

var (
	// errCompressionNotSupported is returned if cannot support current compression type
	errCompressionNotSupported = errors.New("cache:compression: not support this compression type")
)

var compressionAlgoStringMapping = []string{"Snappy", "Gzip", "None"}

var (
	gzipWriterPool = sync.Pool{
		New: func() interface{} {
			return gzip.NewWriter(nil)
		},
	}
	gzipReaderPool sync.Pool
)

var bufferPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

func getBufferFromPool() *bytes.Buffer {
	return bufferPool.Get().(*bytes.Buffer)
}

func putBufferToPool(buf *bytes.Buffer) {
	buf.Reset()
	bufferPool.Put(buf)
}

func (cc AlgoType) String() string {
	if cc < Snappy || cc > None {
		return "Unknown"
	}
	return compressionAlgoStringMapping[int(cc)]
}

/*
// Compressor is the interface of a compressor which support Compress and Decompress method
type Compressor interface {
	// Compress raw bytes into smaller size
	Compress(byt []byte) ([]byte, error)
	// Decompress bytes into original raw bytes
	Decompress(byt []byte) ([]byte, error)
}
*/

// Compress will compress raw bytes into smaller size
func Compress(byt []byte, compressionType AlgoType) ([]byte, error) {
	var b []byte

	switch compressionType {
	case Snappy:
		b = snappy.Encode(nil, byt)
		return b, nil
	case Gzip:
		buf := getBufferFromPool()
		defer putBufferToPool(buf)

		z, _ := gzipWriterPool.Get().(*gzip.Writer)
		defer gzipWriterPool.Put(z)

		z.Reset(buf)
		_, err := z.Write(byt)
		if err != nil {
			return nil, err
		}
		err = z.Close()
		if err != nil {
			return nil, err
		}
		b = buf.Bytes()
		return b, nil
	case None:
		return byt, nil
	}

	return nil, errCompressionNotSupported
}

// Decompress will decompress compressed bytes into original bytes
func Decompress(byt []byte, compressionType AlgoType) ([]byte, error) {
	switch compressionType {
	case Snappy:
		return snappy.Decode(nil, byt)
	case Gzip:
		var reader *gzip.Reader
		var err error
		readerIntf := gzipReaderPool.Get()
		if readerIntf != nil {
			reader, _ = readerIntf.(*gzip.Reader)
		} else {
			reader, err = gzip.NewReader(bytes.NewReader(byt))
			if err != nil {
				return nil, err
			}
		}
		defer gzipReaderPool.Put(reader)
		if err := reader.Reset(bytes.NewReader(byt)); err != nil {
			return nil, err
		}
		return ioutil.ReadAll(reader)
	case None:
		return byt, nil
	}

	return nil, errCompressionNotSupported
}
