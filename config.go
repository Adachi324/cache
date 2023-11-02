package cache

// Type defines cache type used in Config
type Type int

const (
	// Redis is cache type redis
	Redis Type = 1

	// InMemory is cache type in-memory
	InMemory Type = 2

	// MultiLayer is cache type multilayer
	MultiLayer Type = 3
)

// Config defines the configuration for a cache. It is used when initializing or updating cache through manager.
// Based on the Type field, the config used will be from one of the concrete typed field.
// e.g. if Type is Redis, only the Redis field of type RedisConfig will be used.
type Config struct {
	Type Type `yaml:"type" json:"type"`

	//Redis    RedisConfig         `yaml:"redis" json:"redis"`
	InMemory InMemoryCacheConfig `yaml:"in_memory" json:"in_memory"`

	//MultiLayer MultiLayerConfig `yaml:"multilayer" json:"multilayer"`
}

// Validate checks if this Config is valid.
func (c Config) Validate() error {
	switch c.Type {
	//case Redis:
	//	return c.Redis.Validate()
	case InMemory:
		return c.InMemory.Validate()
	//case MultiLayer:
	//	return c.MultiLayer.Validate()
	default:
		return errorConfigTypeNotSupported
	}
}

// RawConfig returns the raw Config of the specific cache type e.g. RedisConfig.
func (c Config) RawConfig() (interface{}, error) {
	switch c.Type {
	//case Redis:
	//	return c.Redis, nil
	case InMemory:
		return c.InMemory, nil
	//case MultiLayer:
	//	return c.MultiLayer, nil
	default:
		return nil, errorConfigTypeNotSupported
	}
}

func (c Config) isSingle() bool {
	switch c.Type {
	case Redis, InMemory:
		return true
	default:
		return false
	}
}

func (c Config) isComposite() bool {
	switch c.Type {
	case MultiLayer:
		return true
	default:
		return false
	}
}

// compositeCacheConfig must be implemented by the composite cache configs.
type compositeCacheConfig interface {
	cacheNames() []string
}
