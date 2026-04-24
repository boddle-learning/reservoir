package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/boddle/reservoir/internal/auth"
	"github.com/boddle/reservoir/internal/config"
	"github.com/boddle/reservoir/internal/database"
	"github.com/boddle/reservoir/internal/middleware"
	"github.com/boddle/reservoir/internal/oauth"
	"github.com/boddle/reservoir/internal/ratelimit"
	"github.com/boddle/reservoir/internal/token"
	"github.com/boddle/reservoir/internal/user"
	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize logger
	var logger *zap.Logger
	if cfg.IsDevelopment() {
		logger, _ = zap.NewDevelopment()
	} else {
		logger, _ = zap.NewProduction()
	}
	defer logger.Sync()

	logger.Info("Starting Boddle Auth Gateway", zap.String("env", cfg.Env))

	// Connect to PostgreSQL
	db, err := database.NewPostgresDB(cfg.Database)
	if err != nil {
		logger.Fatal("Failed to connect to PostgreSQL", zap.Error(err))
	}
	defer db.Close()
	logger.Info("Connected to PostgreSQL")

	// Connect to Redis
	redisClient, err := database.NewRedisClient(cfg.RedisURL)
	if err != nil {
		logger.Fatal("Failed to connect to Redis", zap.Error(err))
	}
	defer redisClient.Close()
	logger.Info("Connected to Redis")

	// Initialize services
	userRepo := user.NewRepository(db.DB)
	tokenService := token.NewService(
		cfg.JWT.SecretKey,
		cfg.JWT.RefreshSecretKey,
		cfg.JWT.AccessTokenTTL,
		cfg.JWT.RefreshTokenTTL,
	)
	tokenBlacklist := token.NewBlacklist(redisClient.Client)
	rateLimiter := ratelimit.NewLimiter(
		redisClient.Client,
		cfg.RateLimit.Window,
		cfg.RateLimit.MaxAttempts,
		cfg.RateLimit.LockoutDuration,
	)
	authService := auth.NewService(userRepo, tokenService, tokenBlacklist, rateLimiter)

	// Global login throttle (token bucket) — protects downstream systems from
	// thundering-herd logins after an outage. Set capacity=0 to disable.
	var globalLoginLimiter middleware.GlobalLoginLimiter
	if cfg.RateLimit.GlobalLoginCapacity > 0 && cfg.RateLimit.GlobalLoginRefill > 0 {
		globalLoginLimiter = ratelimit.NewGlobalLimiter(
			redisClient.Client,
			"ratelimit:global:login",
			cfg.RateLimit.GlobalLoginCapacity,
			cfg.RateLimit.GlobalLoginRefill,
		)
		logger.Info("Global login throttle enabled",
			zap.Int("capacity", cfg.RateLimit.GlobalLoginCapacity),
			zap.Float64("refill_per_sec", cfg.RateLimit.GlobalLoginRefill),
		)
	}

	// Initialize OAuth services
	oauthStateManager := oauth.NewStateManager(redisClient.Client)
	googleService := oauth.NewGoogleService(cfg.Google, oauthStateManager)
	cleverService := oauth.NewCleverService(cfg.Clever, oauthStateManager)

	oauthAuthService := oauth.NewAuthService(userRepo, tokenService, googleService, cleverService)

	// Initialize handlers
	authHandler := auth.NewHandler(authService)
	oauthHandler := oauth.NewHandler(oauthAuthService, googleService, cleverService)

	// Set up Gin router
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()

	// Global middleware
	allowedOrigins := middleware.ParseAllowedOrigins(cfg.CORS.AllowedOrigins)
	router.Use(middleware.CORS(allowedOrigins))
	router.Use(middleware.SecurityHeaders())
	router.Use(middleware.Recovery(logger))
	router.Use(middleware.Logger(logger))
	router.Use(middleware.Metrics())

	// Public routes
	router.GET("/health", authHandler.Health)
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// Auth routes
	authGroup := router.Group("/auth")
	{
		// Global login throttle applies to endpoints that mint new tokens from
		// credentials. Refresh and logout are excluded: refresh flow is already
		// gated by a valid refresh token, and logout must always succeed.
		loginQueue := middleware.LoginQueue(globalLoginLimiter)

		authGroup.POST("/login", loginQueue, authHandler.Login)
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.GET("/token", loginQueue, authHandler.LoginWithToken)
		authGroup.POST("/logout", authHandler.Logout)

		// OAuth routes — throttle the callback (which does the actual auth work
		// and downstream calls), not the redirect initiator.
		authGroup.GET("/google", oauthHandler.GoogleLogin)
		authGroup.GET("/google/callback", loginQueue, oauthHandler.GoogleCallback)
		authGroup.GET("/clever", oauthHandler.CleverLogin)
		authGroup.GET("/clever/callback", loginQueue, oauthHandler.CleverCallback)

		// iCloud route — client sends Apple UID directly, no server-side OAuth flow
		authGroup.POST("/icloud", loginQueue, oauthHandler.ICloudAuth)

		// Protected routes (require authentication)
		authGroup.Use(middleware.Auth(authService))
		{
			authGroup.GET("/me", authHandler.Me)
		}
	}

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		logger.Info("Starting server", zap.String("port", cfg.Port))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shut down the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	// Graceful shutdown with 5 second timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server stopped")
}
