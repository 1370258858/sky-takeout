# orderService

Domain:
- orders
- order_detail
- shopping_cart

Baselines:
- unique order id by redis
- distributed lock with redis and mysql
- MQ producer and timeout consumer
- RPC dependencies to goods/user/delivery
