package api

import _ "embed"

// OpenAPISpec — OpenAPI 2.0 (Swagger) for Ticket API.
//
//go:embed openapi.json
var OpenAPISpec []byte
