# Microservices Scaffold

This folder is the baseline structure for refactoring sky-takeout from monolith to microservices.

Services:
- gatewayService: auth, refresh token, token exchange, routing
- adminService: employee/admin
- userService: user/address_book
- goodsService: category/dish/setmeal
- orderService: orders/order_detail/shopping_cart
- deliveryService: delivery flow
- fileService: file upload (avatar, dish image)
- reportService: statistics and reports
- workspaceService: dashboard and daily data

Security baseline:
- accessToken ttl: 15 minutes
- refreshToken ttl: 7 days

Current phase:
- directory framework prepared
- implementation pending
