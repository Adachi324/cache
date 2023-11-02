package codec

import (
	"fmt"
)

// Config is the config to control default codec type
type Config struct {
	// Type is the default codec type for a cache.
	// Default value is JSON
	// Cache operation level codec set through `WithCodecType` has higher priority than this cache level codec
	Type Type `yaml:"type" json:"type"`
}

// Validate check if config is valid
func (c Config) Validate() error {
	if c.Type < JSON || c.Type > ObjectBuiltInMarshalUnmarshal {
		return fmt.Errorf("cache:invalid_codec_config_default_codec_type: %v", c.Type)
	}
	return nil
}
