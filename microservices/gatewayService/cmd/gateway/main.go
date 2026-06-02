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
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type tokenExchangeRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
	Username     string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
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

var userAuthClient getwayv1.GetwayServiceClient
var userAuthConn *grpc.ClientConn

func main() {
	resources := common.MustInitForService()
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

	// 3) 代理路由
	r.Any("/getway/*proxyPath", proxyHandler)

	

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
func GenerateToken(Username string, subject string, secret string) (string, error) {
	claim := CustomPayload{
		Username:     Username,
		GrantScope: subject,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "Auth_Server",                                   //签发者
			Subject:   subject,                                         //签发对象
			Audience:  jwt.ClaimStrings{"PC", "Wechat_Program"},        //签发受众
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),   //过期时间
			NotBefore: jwt.NewNumericDate(time.Now().Add(time.Second)), //最早使用时间
			IssuedAt:  jwt.NewNumericDate(time.Now()),                  //签发时间
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claim).SignedString([]byte(secret))
	return token, err
}

func ParseToken(token string, secret string) (*CustomPayload, error) {
	// 解析token
	parseToken, err := jwt.ParseWithClaims(token, &CustomPayload{}, func(token *jwt.Token) (i interface{}, err error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	if claims, ok := parseToken.Claims.(*CustomPayload); ok && parseToken.Valid { // 校验token
		return claims, nil
	}
	return nil, errors.New("invalid token")
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
	ctx := context.Background()
	GetAuthRespon, err := userAuthClient.GetAuth(ctx, GetAuthRequest)
	if err != nil {
		retcode.Fatal(c, err, "RPC调用Auth服务失败: "+err.Error())
		return
	}
	if GetAuthRespon.GetSuccess() == false {
		retcode.Fatal(c, errors.New(GetAuthRespon.GetMessage()), "认证失败: "+GetAuthRespon.GetMessage())
		return
	}
	jwttoken,err:=GenerateToken(req.Username,"user",req.Password)
	if err != nil {
		return
	}

	// 返回统一格式
	retcode.OK(c, map[string]any{
		"token":    jwttoken,
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
		retcode.Fatal(c, err, "参数错误: "+err.Error())
		return
	}

	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		retcode.Fatal(c, err, "refreshToken 不能为空")
		return
	}

	// 校验 refreshToken
	parseToken, err := jwt.ParseWithClaims(refreshToken, &CustomPayload{}, func(token *jwt.Token) (i interface{}, err error) {
		return []byte(jwtSecret), nil
	})
	if err != nil || !parseToken.Valid {
		retcode.Fatal(c, err, "无效的 refreshToken")
		return
	}

	//TOKEN 续期逻辑（这里直接返回示例数据，实际项目需要重新生成新的 AccessToken 和 RefreshToken）
	newToken,err:=GenerateToken(req.Username,"user",req.Password)
	if err != nil {
		retcode.Fatal(c, err, "续期失败refreshToken")
		return
	}
	// 返回统一格式
	newJWTToken,err:=ParseToken(newToken,req.Password)
	if err != nil {
		retcode.Fatal(c, err, "无效的 refreshToken")
		return
	}
	retcode.OK(c, map[string]any{
		"token":    newToken,
		"username": req.Username,
		"expireDate":newJWTToken.ExpiresAt.Local().String(),
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
