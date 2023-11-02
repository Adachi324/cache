package cache

import (
	"context"
	"time"

	"go-eCache/codec"
	"go-eCache/internal/compression"
	"go-eCache/internal/group"
)

/**** encodingHandler ****/

type encodingHandler struct {
	compressionAlgo      compression.AlgoType
	minLenForCompression int
	disableEncoding      bool
}

func newEncodingHandler(config EncodingConfig) encodingHandler {
	var minLenForCompression int

	if config.CompressionConfig.MinLenForCompression == 0 {
		minLenForCompression = compression.DefaultMinLenForCompression
	} else {
		minLenForCompression = config.CompressionConfig.MinLenForCompression
	}

	return encodingHandler{
		compressionAlgo:      config.CompressionConfig.CompressionAlgo,
		minLenForCompression: minLenForCompression,
		disableEncoding:      config.DisableEncoding,
	}
}

func (h encodingHandler) encode(byt []byte, opts ...bytesProtocolOption) ([]byte, error) {
	if h.disableEncoding {
		return byt, nil
	}

	option := newProtocolOption()
	for _, opt := range opts {
		opt(option)
	}

	algo := h.compressionAlgo
	if len(byt) < h.minLenForCompression {
		algo = compression.None
	}

	return bytesEncode(byt,
		algo,
		withSoftTimeoutTs(option.softTimeoutTs),
		withHardTimeoutTs(option.hardTimeoutTs))
}

func (h encodingHandler) decode(byt []byte) ([]byte, metaHeader, error) {
	if h.disableEncoding {
		return byt, metaHeader{}, nil
	}

	decodedBytes, header, decodeErr := bytesDecode(byt)
	if decodeErr != nil {
		return nil, metaHeader{}, decodeErr
	}

	return decodedBytes, header, decodeErr
}

/**** codecHandler ****/

type codecHandler struct {
	defaultCodecType codec.Type
}

func newCodecHandler(config codec.Config) codecHandler {
	return codecHandler{config.Type}
}

func (h codecHandler) unmarshal(rawBytes []byte, receiver interface{}, codecType codec.Type, customCodec codec.CustomCodec) error {
	if customCodec != nil {
		return customCodec.Unmarshal(rawBytes, receiver)
	}

	if codecType.Validate() {
		return codec.Unmarshal(rawBytes, receiver, codecType)
	} else if codecType == codec.UnsetCodec {
		return codec.Unmarshal(rawBytes, receiver, h.defaultCodecType)
	}

	return codec.ErrCodecNotSupported
}

func (h codecHandler) marshal(value interface{}, codecType codec.Type, customCodec codec.CustomCodec) ([]byte, error) {
	if customCodec != nil {
		return customCodec.Marshal(value)
	}

	if codecType.Validate() {
		return codec.Marshal(value, codecType)
	} else if codecType == codec.UnsetCodec {
		return codec.Marshal(value, h.defaultCodecType)
	}

	return nil, codec.ErrCodecNotSupported
}

/**** manufacturerHandler ****/

type manufacturerHandler struct {
	strategy                   StampedeMitigationStrategy
	group                      *group.Group // sync results within the same process (instance) if strategy is InProcessSignal or AcrossInstanceSignal.
	acrossInstanceSignalConfig acrossInstanceSignalConfig
}

type acrossInstanceSignalConfig struct {
	retryInterval       time.Duration
	dlockUnitExpiration time.Duration
}

func newManufacturerHandler(config ManufacturerConfig) manufacturerHandler {
	strategy := config.CacheStampedeMitigation
	curManufacturerHandler := manufacturerHandler{strategy: strategy, group: group.NewGroup()}
	if strategy == AcrossInstanceSignal {
		if config.AcrossInstanceSignalConfig.RetryIntervalMillis == 0 {
			config.AcrossInstanceSignalConfig.RetryIntervalMillis = defaultDlockRetryIntervalMillis
		}
		if config.AcrossInstanceSignalConfig.DlockUnitExpirationMillis == 0 {
			config.AcrossInstanceSignalConfig.DlockUnitExpirationMillis = defaultDlockUnitExpirationMillis
		}
		curManufacturerHandler.acrossInstanceSignalConfig = acrossInstanceSignalConfig{
			retryInterval:       time.Duration(config.AcrossInstanceSignalConfig.RetryIntervalMillis) * time.Millisecond,
			dlockUnitExpiration: time.Duration(config.AcrossInstanceSignalConfig.DlockUnitExpirationMillis) * time.Millisecond,
		}
	}
	return curManufacturerHandler
}

// add splits the keys into 2 groups: toHandleKeys and waitingInProcessSignalCallsMap, based on the stampede mitigation strategy.
//
// toHandleKeys: list of keys that needs to be handled by this go-routine.
//
// waitingInProcessSignalCallsMap: list of keys that needs to wait within the same process (instance) by calling `manufacturerHandler::wait()` if strategy is InProcessSignal or AcrossInstanceSignal.
func (h manufacturerHandler) add(ctx context.Context, keys []string) (toHandleKeys []string, waitingInProcessSignalCallsMap map[string]*group.Call) {
	waitingInProcessSignalCallsMap = make(map[string]*group.Call)
	switch h.strategy {
	case NoProtection:
		toHandleKeys = append(toHandleKeys, keys...)
	case InProcessSignal, AcrossInstanceSignal:
		// check whether current request is from shadow traffic
		waitingInProcessSignalCallsMap, toHandleKeys = h.group.AddCalls(keys)

	}
	return toHandleKeys, waitingInProcessSignalCallsMap
}

// complete synchronizes the values in valueMap to the ManufacturerHandler's waiting group.
//
// If the strategy is `NoProtection`, it will do nothing.
func (h manufacturerHandler) complete(ctx context.Context, valueMap map[string]interface{}) {
	switch h.strategy {
	case NoProtection:
	case InProcessSignal, AcrossInstanceSignal:
		h.group.CompleteCalls(valueMap)

	}
}

func genToCompleteResultMap(loadResultMap map[string]loadResult) map[string]interface{} {
	toCompleteResultMap := make(map[string]interface{}, len(loadResultMap))
	for key, result := range loadResultMap {
		toCompleteResultMap[key] = result
	}
	return toCompleteResultMap
}

// wait pends on waiting values, and returns the value and error of the call.
//
// If the strategy is `NoProtection`, it will do nothing.
func (h manufacturerHandler) wait(call *group.Call) (interface{}, error) {
	switch h.strategy {
	case InProcessSignal, AcrossInstanceSignal:
		return h.group.WaitCall(call)
	}
	return nil, nil
}
