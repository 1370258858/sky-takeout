package common

const EnvPrefix = "GOODS_SERVICE"

// MustInitForService initializes mysql, redis, and mq using service env prefix.
func MustInitForService() *Resources {
	cfg := LoadConfigFromEnv(EnvPrefix)
	return MustNewResources(cfg)
}
