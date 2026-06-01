# Architecture Baseline

Gateway:
- validate access token
- refresh access token by refresh token
- token exchange endpoint
- route traffic to downstream services
- reserve middleware slot for rate limiting

Service communication:
- synchronous calls via RPC for order/user/goods/delivery dependencies
- asynchronous calls via MQ for peak shaving and timeout workflows

Order design baseline:
- redis strategy for unique order id generation
- distributed lock implementation with redis and mysql
- timeout order consumer for delayed cancellation
