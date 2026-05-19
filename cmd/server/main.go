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

	// Fail fast if DB_HOST resolves to a reader replica or a read-only role.
	// See PIR 2026-05-19: a reader-pointed DB_HOST silently shipped to prod
	// and broke last_logged_on writes on every auth request for ~31 hours.
	probeCtx, probeCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if err := db.VerifyWritable(probeCtx); err != nil {
		probeCancel()
		logger.Fatal("Database write probe failed", zap.Error(err))
	}
	probeCancel()
	logger.Info("Database write probe passed")

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

	// Background batcher for last_logged_on writes. Started here so the
	// goroutine runs for the lifetime of the process and shuts down with
	// the HTTP server.
	lastLoginWriter := user.NewLastLoginWriter(db.DB, logger)

	authService := auth.NewService(userRepo, tokenService, tokenBlacklist, rateLimiter, lastLoginWriter)

	// Initialize OAuth services
	oauthStateManager := oauth.NewStateManager(redisClient.Client)
	googleService := oauth.NewGoogleService(cfg.Google, oauthStateManager)
	cleverService := oauth.NewCleverService(cfg.Clever, oauthStateManager)

	oauthAuthService := oauth.NewAuthService(userRepo, tokenService, googleService, cleverService, lastLoginWriter)

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
		authGroup.POST("/login", authHandler.Login)
		authGroup.POST("/refresh", authHandler.Refresh)
		authGroup.GET("/token", authHandler.LoginWithToken)
		authGroup.POST("/logout", authHandler.Logout)

		// OAuth token routes: LMS passes pre-obtained OmniAuth tokens for JWT issuance
		authGroup.POST("/google", oauthHandler.GoogleTokenAuth)
		authGroup.POST("/clever", oauthHandler.CleverTokenAuth)

		// OAuth redirect-based routes (Reservoir-led flow)
		authGroup.GET("/google", oauthHandler.GoogleLogin)
		authGroup.GET("/google/callback", oauthHandler.GoogleCallback)
		authGroup.GET("/clever", oauthHandler.CleverLogin)
		authGroup.GET("/clever/callback", oauthHandler.CleverCallback)

		// iCloud route — client sends Apple UID directly, no server-side OAuth flow
		authGroup.POST("/icloud", oauthHandler.ICloudAuth)

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

	// Flush any queued last_logged_on writes before exit. Bounded by ctx
	// (same 5s budget as server shutdown) so a stuck DB cannot stall the
	// process forever.
	lastLoginWriter.Shutdown(ctx)

	logger.Info("Server stopped")
}
