# Common Infrastructure

This folder provides reusable infrastructure bootstrap code for business logic.

Exposed APIs:
- common.LoadConfigFromEnv(prefix)
- common.NewResources(cfg)
- resources.DB()
- resources.Redis()
- resources.MQ()
- resources.PublishJSON(exchange, routingKey, body)
- resources.Close()

Environment variables (service-specific prefix has higher priority):
- <PREFIX>_MYSQL_DSN or MYSQL_DSN
- <PREFIX>_REDIS_ADDR or REDIS_ADDR
- <PREFIX>_REDIS_PASSWORD or REDIS_PASSWORD
- <PREFIX>_REDIS_DB or REDIS_DB
- <PREFIX>_MQ_URL or MQ_URL