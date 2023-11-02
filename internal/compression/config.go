package compression

import (
	"fmt"
)

// Config is the config of algorithm used to compress the bytes before sending to external cache and decompress to the original bytes after receiving from external cache
// Different compression algorithms will perform differently in the compression ratio and CPU cost for compress/decompress
// Default value is 0, meaning it will use `Snappy`. Moreover, in this config, users can also decide the minimal compression length of original raw bytes
// If the length of raw bytes is less than this configured length, it will not do compression for it.
type Config struct {
	// CompressionAlgo could be Snappy (Default), Gzip or None
	CompressionAlgo AlgoType `yaml:"compression_algo" json:"compression_algo"`
	// MinLenForCompression is the minimal raw bytes length in which case we will do compression. Default value is 512
	MinLenForCompression int `yaml:"min_len_for_compression" json:"min_len_for_compression"`
}

// Validate check if config is valid
func (c *Config) Validate() error {
	if c != nil {
		if c.CompressionAlgo < Snappy || c.CompressionAlgo > None {
			return fmt.Errorf("cache:invalid_compression_config_compression_algo: %v", c.CompressionAlgo)
		}
		if c.MinLenForCompression < 0 {
			return fmt.Errorf("cache:invalid_compression_config_min_len_for_compression: %v", c.MinLenForCompression)
		}
	}
	return nil
}
