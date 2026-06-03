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
	getwayv1 "sky-takeout/microservices/gatewayService/rpc/pb"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type tokenExchangeRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	AccessToken  string `json:"accessToken" binding:"required"`
	RefreshToken string `json:"refreshToken" binding:"required"`
	Username     string `json:"username" binding:"required"`
	Password     string `json:"password" binding:"required"`
}

type refreshResponse struct {
	AccessToken            string `json:"accessToken" binding:"required"`
	AccessTokenexpireDate  string `json:"accessTokenExpireDate" binding:"required"`
	RefreshToken           string `json:"refreshToken" binding:"required"`
	RefreshTokenexpireDate string `json:"refreshTokenExpireDate" binding:"required"`
	Username               string `json:"username" binding:"required"`
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
var jwtSecret = "your-gateway-jwt-secret"

var userAuthClient getwayv1.GetwayServiceClient
var userAuthConn *grpc.ClientConn
var rd *redis.Client
var gatewayResources *common.Resources

// RD returns the initialized redis client for gateway features.
func RD() *redis.Client {
	if rd == nil {
		log.Fatal("gateway redis client is not initialized")
	}
	return rd
}

func main() {
	resources := common.MustInitForService()
	gatewayResources = resources
	rd = resources.Redis()
	if rd == nil {
		log.Fatal("gateway redis client init failed: nil client")
	}
	defer func() {
		if err := resources.Close(); err != nil {
			log.Printf("gatewayService close resources error: %v", err)
		}
	}()

	userServiceAddr := os.Getenv("USER_SERVICE_ADDR")
	if userServiceAddr == "" {
		userServiceAddr = "user-service:19082"
	}

	conn, err := grpc.Dial(userServiceAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("gatewayService connect userService error: %v", err)
	}
	userAuthConn = conn
	defer func() {
		if err := userAuthConn.Close(); err != nil {
			log.Printf("gatewayService close user grpc conn error: %v", err)
		}
	}()
	userAuthClient = getwayv1.NewGetwayServiceClient(userAuthConn)

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
	r.POST("/getway/token/exchange", tokenExchangeHandler)

	// 2) 刷新 token
	r.POST("/getway/token/refresh", tokenRefreshHandler)

	// 3) 强制下线
	r.POST("/getway/token/revoke", tokenRevokeHandler)
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

// GenerateToken 生成Token uid 用户id subject 签发对象  secret 加盐
func GenerateToken(Username string, subject string, secret string, expiresMin int64) (string, error) {
	claim := CustomPayload{
		Username:   Username,
		GrantScope: subject,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "Auth_Server",                                                               //签发者
			Subject:   subject,                                                                     //签发对象
			Audience:  jwt.ClaimStrings{"PC", "Wechat_Program"},                                    //签发受众
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute * time.Duration(expiresMin))), //过期时间
			NotBefore: jwt.NewNumericDate(time.Now()),                                              //最早使用时间
			IssuedAt:  jwt.NewNumericDate(time.Now()),                                              //签发时间
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claim).SignedString([]byte(secret))
	return token, err
}

func ParseToken(tokenString string, secret string) (*jwt.Token, error) {

	token, err := jwt.ParseWithClaims(
		tokenString,
		&CustomPayload{},
		func(token *jwt.Token) (interface{}, error) {
			return []byte(secret), nil
		},
	)

	if token == nil {
		return nil, err
	}
	return token, err
}

// 判断token是否临近过期
func IsTokenExpiringSoon(token *jwt.Token, threshold time.Duration) (bool, error) {
	claims, ok := token.Claims.(*CustomPayload)
	if !ok {
		return false, errors.New("invalid token claims")
	}
	if claims.ExpiresAt == nil {
		return false, errors.New("token does not have an expiration time")
	}
	expirationTime := claims.ExpiresAt.Time
	return time.Until(expirationTime) <= threshold, nil
}

// ======================
// 兑换 Token 处理器
// ======================
func tokenExchangeHandler(c *gin.Context) {
	var req tokenExchangeRequest
	// Gin 自动绑定 JSON 参数
	if err := c.ShouldBindJSON(&req); err != nil {
		retcode.Fatal(c, err, "参数错误: "+err.Error())
		return
	}
	// TODO: 调用 RPC Auth 服务校验用户名密码
	GetAuthRequest := &getwayv1.GetAuthRequest{
		UserName: req.Username,
		Password: req.Password,
	}
	GetAuthRespon, err := userAuthClient.GetAuth(c, GetAuthRequest)
	if err != nil {
		retcode.Fatal(c, err, "RPC调用Auth服务失败: "+err.Error())
		return
	}
	if GetAuthRespon.GetSuccess() == false {
		retcode.Fatal(c, errors.New(GetAuthRespon.GetMessage()), "认证失败: "+GetAuthRespon.GetMessage())
		return
	}
	accessToken, err := GenerateToken(req.Username, "user", jwtSecret, 5)
	if err != nil {
		retcode.Fatal(c, err, "生成accessToken失败: "+err.Error())
		return
	}
	refreshToken, err := GenerateToken(req.Username, "user", jwtSecret, 15)
	if err != nil {
		retcode.Fatal(c, err, "生成refreshToken失败: "+err.Error())
		return
	}

	//"refreshToken"+req.Username  作为key，refreshToken 作为value，过期时间设置为15分钟
	rdRes := RD().Set(c, "refreshToken-"+req.Username, refreshToken, 15*time.Minute)
	if rdRes.Err() != nil {
		log.Printf("gatewayService set refresh token to redis error: %v", rdRes.Err())
	}
	var tokenResponse = refreshResponse{
		Username:     req.Username,
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}

	// 返回统一格式
	retcode.OK(c, tokenResponse)
}

// ======================
// 刷新 Token 处理器
// 只刷新access token
// ======================
func tokenRefreshHandler(c *gin.Context) {
	var req refreshRequest
	var err error
	if err := c.ShouldBindJSON(&req); err != nil {
		retcode.Fatal(c, err, "参数错误: "+err.Error())
		return
	}

	accessTokenString := strings.TrimSpace(req.AccessToken)
	if accessTokenString == "" {
		retcode.Fatal(c, err, "accessToken 不能为空")
		return
	}

	// 校验 accessToken
	accessToken, err := ParseToken(accessTokenString, jwtSecret)
	if err != nil {
		retcode.Fatal(c, err, "accessToken 无效: "+err.Error())
		return
	}
	// 这里我们允许过期的 accessToken 进行刷新，所以不检查accessToken.Valid
	if accessToken == nil || !accessToken.Valid {
		retcode.Fatal(c, err, "accessToken 无效")
		return
	}
	refreshTokenInRedis, err := RD().Get(c, "refreshToken-"+req.Username).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			retcode.Fatal(c, errors.New("refreshToken 不存在或已过期"), "获取 refreshToken 失败")
			return
		}
		retcode.Fatal(c, err, "refreshToken 不存在或已过期 ,获取 refreshToken 失败: "+err.Error())
		return
	}
	if refreshTokenInRedis != req.RefreshToken {
		retcode.Fatal(c, errors.New("refreshToken 与服务器记录不一致"), "refreshToken 无效")
		return
	}
	//accesstoken 临近过期时间未3分钟，才允许刷新
	//refleshToken 续期逻辑（这里直接返回旧的 refreshToken，实际项目需要重新生成新的 RefreshToken）
	if IsExpiring, err := IsTokenExpiringSoon(accessToken, 3*time.Minute); err != nil {
		retcode.Fatal(c, err, "检查 accessToken 是否临近过期失败: "+err.Error())
		return
	} else if !IsExpiring {
		retcode.Fatal(c, errors.New("accessToken 没有临近过期"), "accessToken 没有临近过期，无需刷新")
		return
	}

	//TOKEN 续期逻辑 （这里直接生成新的 accessToken，实际项目可能需要更复杂的续期逻辑，比如同时续期 refreshToken）
	newAccessToken, err := GenerateToken(req.Username, "user", jwtSecret, 5)
	if err != nil {
		retcode.Fatal(c, err, "续期失败accessToken")
		return
	}

	// 返回统一格式
	newJWTToken, err := ParseToken(newAccessToken, jwtSecret)
	if err != nil {
		retcode.Fatal(c, err, "老token解析通过，新token是无效的 accessToken")
		return
	}
	time, _ := newJWTToken.Claims.GetExpirationTime()
	var refreshResponse = refreshResponse{
		AccessToken:            newAccessToken,
		AccessTokenexpireDate:  time.Local().String(),
		RefreshToken:           req.RefreshToken, // 刷新 token 续期逻辑（这里直接返回旧的 refreshToken，实际项目需要重新生成新的 RefreshToken）
		RefreshTokenexpireDate: "暂时不展示",
		Username:               req.Username,
	}

	retcode.OK(c, refreshResponse)

}

func tokenRevokeHandler(c *gin.Context) {
	var req refreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		retcode.Fatal(c, err, "参数错误: "+err.Error())
		return
	}
	if strings.TrimSpace(req.Username) == "" {
		retcode.Fatal(c, errors.New("username 不能为空"), "username 不能为空")
		return
	}
	parseToken, err := ParseToken(req.AccessToken, jwtSecret)
	if err != nil || parseToken == nil || !parseToken.Valid {
		retcode.Fatal(c, err, "accessToken 无效: "+err.Error())
		return
	}
	rdRes := RD().Del(c, "refreshToken-"+req.Username)
	if rdRes.Err() != nil {
		retcode.Fatal(c, rdRes.Err(), "删除 refreshToken 失败: "+rdRes.Err().Error())
		return
	}
	retcode.OK(c, map[string]any{
		"message": "token revoked",
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
