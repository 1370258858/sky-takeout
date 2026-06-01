package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sky-takeout/microservices/gatewayService/common"
	"sky-takeout/microservices/gatewayService/common/retcode"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type tokenExchangeRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

type tokenPair struct {
	AccessToken           string `json:"accessToken"`
	RefreshToken          string `json:"refreshToken"`
	AccessTokenExpiresIn  int    `json:"accessTokenExpiresIn"`
	RefreshTokenExpiresIn int    `json:"refreshTokenExpiresIn"`
}

// CustomPayload 自定义载荷
type CustomPayload struct {
	GrantScope string
	Username   string `json:"username"`
	jwt.RegisteredClaims
}

type EmployeeLogin struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token"`
}

// 全局 JWT 密钥（真实项目必须放在配置/环境变量）
var jwtSecret = []byte("your-gateway-jwt-secret")

func main() {
	resources := common.MustInitForService()
	defer func() {
		if err := resources.Close(); err != nil {
			log.Printf("gatewayService close resources error: %v", err)
		}
	}()

	// ========== 替换成 GIN ==========
	r := gin.Default()

	// 健康检查
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, map[string]any{
			"service": "gatewayService",
			"status":  "ok",
		})
	})

	// 1) 兑换 token
	r.POST("/auth/token/exchange", tokenExchangeHandler)

	// 2) 刷新 token
	r.POST("/auth/token/refresh", tokenRefreshHandler)

	// 3) 代理路由
	r.Any("/proxy/*proxyPath", proxyHandler)

	// ========== 服务启动 ==========
	addr := ":18080"
	server := &http.Server{
		Addr:    addr,
		Handler: r,
	}

	go func() {
		log.Printf("gatewayService listening on %s (gin mode)", addr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("gatewayService serve error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("gatewayService shutdown error: %v", err)
	}
}

// ======================
// 兑换 Token 处理器
// ======================
func tokenExchangeHandler(c *gin.Context) {
	var req tokenExchangeRequest
	// Gin 自动绑定 JSON 参数
	if err := c.ShouldBindJSON(&req); err != nil {
		retcode.Fatal(c, err,"参数错误: "+err.Error())
		return
	}

	// TODO: 调用 RPC Auth 服务校验用户名密码
	// 这里先模拟校验成功

	// 构造 JWT 载荷
	claims := CustomPayload{
		Username:   req.Username,
		GrantScope: "subject",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "Auth_Server",
			Subject:   "user",
			Audience:  jwt.ClaimStrings{"PC", "Wechat_Program"},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
			NotBefore: jwt.NewNumericDate(time.Now()),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	// 生成 Token（密钥不能用密码！）
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret)
	if err != nil {
		retcode.Fatal(c, err, "生成Token失败")
		return
	}

	// 返回统一格式
	retcode.OK(c, map[string]any{
		"token":    token,
		"username": req.Username,
	})
}

// ======================
// 刷新 Token 处理器
// ======================
func tokenRefreshHandler(c *gin.Context) {
	var req refreshRequest
	var err error
	if err := c.ShouldBindJSON(&req); err != nil {
		retcode.Fatal(c, err,"参数错误: "+err.Error())
		return
	}

	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		retcode.Fatal(c, err,"refreshToken 不能为空")
		return
	}

	// TODO: 校验 refreshToken
	// 这里返回示例
	c.JSON(http.StatusOK, map[string]any{
		"accessToken":          "demo-new-access-token",
		"accessTokenExpiresIn": 15 * 60,
		"issuedAt":             time.Now().Unix(),
	})
}

// ======================
// 代理处理器
// ======================
func proxyHandler(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, map[string]any{
		"message": "proxy route placeholder",
		"path":    c.Request.URL.Path,
	})
}