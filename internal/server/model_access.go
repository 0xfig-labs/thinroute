package server

import "github.com/icehugh/thinroute/internal/gateway"

// RequestModelAuthorizer validates request-scoped access to concrete models.
type RequestModelAuthorizer = gateway.ModelAuthorizer
