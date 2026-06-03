package main

import (
	"context"
	"errors"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sky-takeout/microservices/userService/common"
	getwayv1 "sky-takeout/microservices/userService/rpc/pb"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
)

type authService struct {
	getwayv1.UnimplementedGetwayServiceServer
}

func (s *authService) GetAuth(_ context.Context, req *getwayv1.GetAuthRequest) (*getwayv1.GetAuthResponse, error) {
	username := strings.TrimSpace(req.GetUserName())
	password := strings.TrimSpace(req.GetPassword())

	if username == "" || password == "" {
		return &getwayv1.GetAuthResponse{
			Success: false,
			Message: "用户名或密码不能为空",
		}, nil
	}

	// TODO: replace with real user credential verification.
	return &getwayv1.GetAuthResponse{
		Success: true,
		Message: "认证成功",
	}, nil
}

func main() {
	resources := common.MustInitForService()
	defer func() {
		if err := resources.Close(); err != nil {
			log.Printf("userService close resources error: %v", err)
		}
	}()

	r := gin.Default()
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, map[string]any{
			"service": "userService",
			"status":  "ok",
		})
	})

	httpAddr := ":18082"
	httpServer := &http.Server{Addr: httpAddr, Handler: r}

	grpcAddr := os.Getenv("USER_SERVICE_GRPC_ADDR")
	if strings.TrimSpace(grpcAddr) == "" {
		grpcAddr = ":19082"
	}

	grpcListener, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("userService listen grpc error: %v", err)
	}

	grpcServer := grpc.NewServer()
	getwayv1.RegisterGetwayServiceServer(grpcServer, &authService{})

	go func() {
		log.Printf("userService listening on %s (gin mode)", httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("userService serve error: %v", err)
		}
	}()

	go func() {
		log.Printf("userService grpc listening on %s", grpcAddr)
		if err := grpcServer.Serve(grpcListener); err != nil {
			log.Fatalf("userService grpc serve error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("userService shutdown error: %v", err)
	}
	grpcServer.GracefulStop()
}
