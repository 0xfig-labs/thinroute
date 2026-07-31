package server

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/icehugh/thinroute/internal/control"
)

// ControlServer exposes runtime diagnostics and actions separately from the
// public inference server.
type ControlServer struct {
	echo *echo.Echo
}

func NewControlServer(handler *control.Handler, token string) *ControlServer {
	e := echo.New()
	if token = strings.TrimSpace(token); token != "" {
		e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c *echo.Context) error {
				got := strings.TrimPrefix(c.Request().Header.Get("Authorization"), "Bearer ")
				if subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
					return c.JSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
				}
				return next(c)
			}
		})
	}
	handler.RegisterRoutes(e.Group("/control/v1"))
	return &ControlServer{echo: e}
}

func (s *ControlServer) Start(ctx context.Context, addr string) error {
	return newGatewayStartConfig(addr).Start(ctx, s.echo)
}

func (s *ControlServer) StartWithListener(ctx context.Context, listener net.Listener) error {
	return echo.StartConfig{HideBanner: true, Listener: listener}.Start(ctx, s.echo)
}
