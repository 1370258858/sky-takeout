package common

const EnvPrefix = "USER_SERVICE"

// MustInitForService initializes mysql, redis, and mq using service env prefix.
func MustInitForService() *Resources {
	cfg := LoadConfigFromEnv(EnvPrefix)
	return MustNewResources(cfg)
}
