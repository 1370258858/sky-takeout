package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"sky-takeout/microservices/gatewayService/common"
)

type tokenExchangeRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type tokenPair struct {
	AccessToken           string `json:"accessToken"`
	RefreshToken          string `json:"refreshToken"`
	AccessTokenExpiresIn  int    `json:"accessTokenExpiresIn"`
	RefreshTokenExpiresIn int    `json:"refreshTokenExpiresIn"`
}

func main() {
	resources := common.MustInitForService()
	defer func() {
		if err := resources.Close(); err != nil {
			log.Printf("gatewayService close resources error: %v", err)
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"service": "gatewayService", "status": "ok"})
	})

	// 1) 兑换 token
	mux.HandleFunc("/auth/token/exchange", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}

		var req tokenExchangeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.Username) == "" || strings.TrimSpace(req.Password) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "username/password required"})
			return
		}

		writeJSON(w, http.StatusOK, tokenPair{
			AccessToken:           "demo-access-token",
			RefreshToken:          "demo-refresh-token",
			AccessTokenExpiresIn:  15 * 60,
			RefreshTokenExpiresIn: 7 * 24 * 60 * 60,
		})
	})

	// 2) 刷新 token
	mux.HandleFunc("/auth/token/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
			return
		}

		var req refreshRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid request body"})
			return
		}
		if strings.TrimSpace(req.RefreshToken) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "refreshToken required"})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"accessToken":          "demo-new-access-token",
			"accessTokenExpiresIn": 15 * 60,
			"issuedAt":             time.Now().Unix(),
		})
	})

	// 3) 分流到其他服务（占位）
	mux.HandleFunc("/proxy/", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusNotImplemented, map[string]any{
			"message": "proxy route placeholder",
			"path":    r.URL.Path,
		})
	})

	addr := ":18080"
	server := &http.Server{Addr: addr, Handler: mux}

	go func() {
		log.Printf("gatewayService listening on %s", addr)
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

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
