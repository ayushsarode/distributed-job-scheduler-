package api

import (
	"net/http"
	"strings"

	"exiro.ai/application/auth"
	"exiro.ai/application/service/types"
	"github.com/go-chi/traceid"
	"github.com/rs/zerolog"
)

// newAuthMiddleware verifies the authentication token and sets the user ID in context.
func (s *Server) newAuthMiddleware(excludedPaths []string, tokenVerifier auth.TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := zerolog.Ctx(ctx)
			// Check if path is excluded from authentication
			for _, path := range excludedPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					logger.Debug().Ctx(ctx).Msgf("Path %s is excluded from authentication", r.URL.Path)
					next.ServeHTTP(w, r)
					return
				}
			}

			// Verify Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Debug().Ctx(ctx).Msg("Missing Authorization header")
				http.Error(w, "Authorization header required", http.StatusUnauthorized)
				return
			}

			// Extract Bearer token
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if token == "" || token == authHeader {
				logger.Debug().Ctx(ctx).Msg("Invalid Authorization token format")
				http.Error(w, "Invalid Authorization token", http.StatusUnauthorized)
				return
			}

			// Verify token
			userID, err := tokenVerifier.Verify(token)
			if err != nil {
				logger.Error().Ctx(ctx).Err(err).Msg("Token verification failed")
				http.Error(w, "Invalid authentication token", http.StatusUnauthorized)
				return
			}

			// Set user ID in context
			ctx = auth.SetUser(ctx, userID)
			r = r.WithContext(ctx)

			// Proceed with the request
			next.ServeHTTP(w, r)
		})
	}
}

// newTenantMiddleware fetches the tenant ID for the authenticated user and sets it in context.
// It requires that the user ID is already set in context (from auth middleware).
func (s *Server) newTenantMiddleware(excludedPaths []string, usermanagementservice types.UserManagementService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			logger := zerolog.Ctx(ctx)

			// Check if path is excluded from tenant check
			for _, path := range excludedPaths {
				if strings.HasPrefix(r.URL.Path, path) {
					logger.Debug().Ctx(ctx).Msgf("Path %s is excluded from tenant check", r.URL.Path)
					next.ServeHTTP(w, r)
					return
				}
			}

			// Get user ID from context (must be set by auth middleware)
			userID, err := auth.GetUser(r.Context())
			if err != nil {
				logger.Error().Ctx(ctx).Err(err).Msg("User ID not found in context")
				http.Error(w, "Authentication required", http.StatusUnauthorized)
				return
			}

			// Fetch tenant ID for the user
			tenantID, err := usermanagementservice.GetUserTenantID(r.Context(), userID)
			if err != nil {
				logger.Error().Ctx(ctx).Err(err).Msg("Failed to get tenant for user")
				http.Error(w, "User not associated with tenant", http.StatusForbidden)
				return
			}

			// Set tenant ID in context
			ctx = auth.SetTenant(ctx, tenantID)
			r = r.WithContext(ctx)

			// Proceed with the request
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) traceLogMiddleware(logger *zerolog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			trace := traceid.FromContext(r.Context())
			l := logger.With().
				Str("trace_id", trace).
				Logger()
			ctx := l.WithContext(r.Context())
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
