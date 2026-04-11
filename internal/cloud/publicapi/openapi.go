package publicapi

import "strings"

// OpenAPISpec returns the OpenAPI 3.0 specification for the public API.
// This is generated from the proto service definitions and route table.
// Once buf generate is wired (KAI-310), this will be replaced by the
// auto-generated openapi.json from grpc-gateway annotations.
func OpenAPISpec() string {
	var b strings.Builder
	b.WriteString(`{
  "openapi": "3.0.3",
  "info": {
    "title": "KaiVue VMS Public API",
    "description": "Public REST API for the KaiVue Video Management System. Provides full CRUD operations on cameras, users, recordings, events, schedules, retention policies, and integrations.",
    "version": "1.0.0",
    "contact": {
      "name": "KaiVue API Support",
      "url": "https://docs.kaivue.io/api"
    },
    "license": {
      "name": "Proprietary"
    }
  },
  "servers": [
    {
      "url": "https://api.kaivue.io/api/v1",
      "description": "Production"
    },
    {
      "url": "https://api-staging.kaivue.io/api/v1",
      "description": "Staging"
    }
  ],
  "security": [
    {"ApiKeyAuth": []},
    {"BearerAuth": []}
  ],
  "components": {
    "securitySchemes": {
      "ApiKeyAuth": {
        "type": "apiKey",
        "in": "header",
        "name": "X-API-Key",
        "description": "API key for service-to-service and integration authentication. Format: kvue_<40 hex chars>"
      },
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT",
        "description": "OAuth 2.0 bearer token from the KaiVue identity provider"
      }
    },
    "schemas": {
      "Error": {
        "type": "object",
        "properties": {
          "code": {"type": "string", "description": "Machine-readable error code"},
          "message": {"type": "string", "description": "Human-readable error message"},
          "request_id": {"type": "string", "description": "Request ID for support correlation"}
        },
        "required": ["code", "message"]
      },
      "Pagination": {
        "type": "object",
        "properties": {
          "next_cursor": {"type": "string"},
          "total_count": {"type": "integer", "format": "int32"}
        }
      },`)

	// Cameras
	b.WriteString(`
      "Camera": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "name": {"type": "string"},
          "description": {"type": "string"},
          "manufacturer": {"type": "string"},
          "model": {"type": "string"},
          "firmware_version": {"type": "string"},
          "mac_address": {"type": "string"},
          "ip_address": {"type": "string"},
          "status": {"type": "string", "enum": ["online", "offline", "disabled", "error"]},
          "labels": {"type": "array", "items": {"type": "string"}},
          "recording_mode": {"type": "string"},
          "recorder_id": {"type": "string"},
          "created_at": {"type": "string", "format": "date-time"},
          "updated_at": {"type": "string", "format": "date-time"},
          "last_seen_at": {"type": "string", "format": "date-time"}
        }
      },`)

	// Users
	b.WriteString(`
      "User": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "username": {"type": "string"},
          "email": {"type": "string", "format": "email"},
          "display_name": {"type": "string"},
          "roles": {"type": "array", "items": {"type": "string"}},
          "groups": {"type": "array", "items": {"type": "string"}},
          "disabled": {"type": "boolean"},
          "created_at": {"type": "string", "format": "date-time"},
          "updated_at": {"type": "string", "format": "date-time"},
          "last_login_at": {"type": "string", "format": "date-time"}
        }
      },`)

	// Recordings
	b.WriteString(`
      "Recording": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "camera_id": {"type": "string"},
          "camera_name": {"type": "string"},
          "start_time": {"type": "string", "format": "date-time"},
          "end_time": {"type": "string", "format": "date-time"},
          "bytes": {"type": "integer", "format": "int64"},
          "codec": {"type": "string"},
          "has_audio": {"type": "boolean"},
          "is_event_clip": {"type": "boolean"},
          "storage_tier": {"type": "string"},
          "recorder_id": {"type": "string"}
        }
      },`)

	// Events
	b.WriteString(`
      "Event": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "camera_id": {"type": "string"},
          "camera_name": {"type": "string"},
          "kind": {"type": "string"},
          "kind_label": {"type": "string"},
          "severity": {"type": "string", "enum": ["info", "warning", "critical"]},
          "confidence": {"type": "number", "format": "float"},
          "occurred_at": {"type": "string", "format": "date-time"},
          "thumbnail_url": {"type": "string"},
          "recording_id": {"type": "string"},
          "attributes": {"type": "object", "additionalProperties": {"type": "string"}},
          "acknowledged": {"type": "boolean"},
          "acknowledged_by": {"type": "string"},
          "acknowledged_at": {"type": "string", "format": "date-time"}
        }
      },`)

	// Schedules
	b.WriteString(`
      "Schedule": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "name": {"type": "string"},
          "description": {"type": "string"},
          "timezone": {"type": "string"},
          "entries": {"type": "array", "items": {"$ref": "#/components/schemas/ScheduleEntry"}},
          "camera_ids": {"type": "array", "items": {"type": "string"}},
          "created_at": {"type": "string", "format": "date-time"},
          "updated_at": {"type": "string", "format": "date-time"}
        }
      },
      "ScheduleEntry": {
        "type": "object",
        "properties": {
          "day_of_week": {"type": "integer", "minimum": 1, "maximum": 7},
          "start_minute": {"type": "integer"},
          "end_minute": {"type": "integer"},
          "recording_mode": {"type": "string"}
        }
      },`)

	// Retention
	b.WriteString(`
      "RetentionPolicy": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "name": {"type": "string"},
          "description": {"type": "string"},
          "retention_days": {"type": "integer", "format": "int32"},
          "max_bytes": {"type": "integer", "format": "int64"},
          "event_retention_days": {"type": "integer", "format": "int32"},
          "camera_ids": {"type": "array", "items": {"type": "string"}},
          "created_at": {"type": "string", "format": "date-time"},
          "updated_at": {"type": "string", "format": "date-time"}
        }
      },`)

	// Integrations
	b.WriteString(`
      "Integration": {
        "type": "object",
        "properties": {
          "id": {"type": "string"},
          "name": {"type": "string"},
          "description": {"type": "string"},
          "kind": {"type": "string", "enum": ["webhook", "siem", "access_control", "cloud_storage", "analytics", "alarm_panel", "vms", "custom"]},
          "status": {"type": "string", "enum": ["active", "disabled", "error"]},
          "config_json": {"type": "string"},
          "endpoint_url": {"type": "string"},
          "event_filters": {"type": "array", "items": {"type": "string"}},
          "camera_ids": {"type": "array", "items": {"type": "string"}},
          "created_at": {"type": "string", "format": "date-time"},
          "updated_at": {"type": "string", "format": "date-time"},
          "last_triggered_at": {"type": "string", "format": "date-time"},
          "last_error": {"type": "string"}
        }
      }
    },
    "parameters": {
      "PageSize": {
        "name": "page_size",
        "in": "query",
        "schema": {"type": "integer", "minimum": 1, "maximum": 100, "default": 25}
      },
      "Cursor": {
        "name": "cursor",
        "in": "query",
        "schema": {"type": "string"}
      }
    }
  },`)

	// Paths
	b.WriteString(`
  "paths": {`)

	// Camera paths
	writeCRUDPaths(&b, "cameras", "Camera",
		"Manage cameras in your VMS deployment")

	// User paths
	writeCRUDPaths(&b, "users", "User",
		"Manage users in your tenant")

	// Recording paths
	writeReadDeletePaths(&b, "recordings", "Recording",
		"Query and manage recorded video segments")

	// Event paths
	writeEventPaths(&b)

	// Schedule paths
	writeCRUDPaths(&b, "schedules", "Schedule",
		"Manage recording schedules")

	// Retention policy paths
	writeCRUDPaths(&b, "retention-policies", "RetentionPolicy",
		"Manage retention policies")

	// Integration paths
	writeIntegrationPaths(&b)

	// Health
	b.WriteString(`
    "/health": {
      "get": {
        "summary": "Health check",
        "operationId": "healthCheck",
        "tags": ["system"],
        "security": [],
        "responses": {
          "200": {
            "description": "API is healthy",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "status": {"type": "string"},
                    "version": {"type": "string"}
                  }
                }
              }
            }
          }
        }
      }
    }
  },
  "tags": [
    {"name": "cameras", "description": "Camera management"},
    {"name": "users", "description": "User management"},
    {"name": "recordings", "description": "Recording management"},
    {"name": "events", "description": "Event management"},
    {"name": "schedules", "description": "Schedule management"},
    {"name": "retention", "description": "Retention policy management"},
    {"name": "integrations", "description": "Integration management"},
    {"name": "system", "description": "System endpoints"}
  ]
}`)

	return b.String()
}

func writeCRUDPaths(b *strings.Builder, resource, schema, desc string) {
	tag := resource
	// Collection endpoint
	b.WriteString(`
    "/` + resource + `": {
      "get": {
        "summary": "List ` + resource + `",
        "operationId": "list` + schema + `s",
        "tags": ["` + tag + `"],
        "description": "` + desc + `",
        "parameters": [
          {"$ref": "#/components/parameters/PageSize"},
          {"$ref": "#/components/parameters/Cursor"},
          {"name": "search", "in": "query", "schema": {"type": "string"}}
        ],
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "` + resource + `": {"type": "array", "items": {"$ref": "#/components/schemas/` + schema + `"}},
                    "pagination": {"$ref": "#/components/schemas/Pagination"}
                  }
                }
              }
            }
          },
          "401": {"description": "Unauthenticated", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}},
          "429": {"description": "Rate limited", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}}
        }
      },
      "post": {
        "summary": "Create ` + singularize(resource) + `",
        "operationId": "create` + schema + `",
        "tags": ["` + tag + `"],
        "requestBody": {
          "required": true,
          "content": {"application/json": {"schema": {"$ref": "#/components/schemas/` + schema + `"}}}
        },
        "responses": {
          "201": {"description": "Created", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/` + schema + `"}}}},
          "400": {"description": "Invalid request", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}},
          "401": {"description": "Unauthenticated", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}}
        }
      }
    },
    "/` + resource + `/{id}": {
      "get": {
        "summary": "Get ` + singularize(resource) + `",
        "operationId": "get` + schema + `",
        "tags": ["` + tag + `"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "200": {"description": "Successful response", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/` + schema + `"}}}},
          "404": {"description": "Not found", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}}
        }
      },
      "patch": {
        "summary": "Update ` + singularize(resource) + `",
        "operationId": "update` + schema + `",
        "tags": ["` + tag + `"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "requestBody": {
          "required": true,
          "content": {"application/json": {"schema": {"$ref": "#/components/schemas/` + schema + `"}}}
        },
        "responses": {
          "200": {"description": "Updated", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/` + schema + `"}}}},
          "400": {"description": "Invalid request", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}},
          "404": {"description": "Not found", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}}
        }
      },
      "delete": {
        "summary": "Delete ` + singularize(resource) + `",
        "operationId": "delete` + schema + `",
        "tags": ["` + tag + `"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "204": {"description": "Deleted"},
          "404": {"description": "Not found", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Error"}}}}
        }
      }
    },`)
}

func writeReadDeletePaths(b *strings.Builder, resource, schema, desc string) {
	tag := resource
	b.WriteString(`
    "/` + resource + `": {
      "get": {
        "summary": "List ` + resource + `",
        "operationId": "list` + schema + `s",
        "tags": ["` + tag + `"],
        "description": "` + desc + `",
        "parameters": [
          {"$ref": "#/components/parameters/PageSize"},
          {"$ref": "#/components/parameters/Cursor"},
          {"name": "camera_id", "in": "query", "schema": {"type": "string"}},
          {"name": "start_time", "in": "query", "schema": {"type": "string", "format": "date-time"}},
          {"name": "end_time", "in": "query", "schema": {"type": "string", "format": "date-time"}}
        ],
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "` + resource + `": {"type": "array", "items": {"$ref": "#/components/schemas/` + schema + `"}},
                    "pagination": {"$ref": "#/components/schemas/Pagination"}
                  }
                }
              }
            }
          }
        }
      }
    },
    "/` + resource + `/{id}": {
      "get": {
        "summary": "Get ` + singularize(resource) + `",
        "operationId": "get` + schema + `",
        "tags": ["` + tag + `"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "200": {"description": "Successful response", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/` + schema + `"}}}}
        }
      },
      "delete": {
        "summary": "Delete ` + singularize(resource) + `",
        "operationId": "delete` + schema + `",
        "tags": ["` + tag + `"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "204": {"description": "Deleted"}
        }
      }
    },`)
}

func writeEventPaths(b *strings.Builder) {
	b.WriteString(`
    "/events": {
      "get": {
        "summary": "List events",
        "operationId": "listEvents",
        "tags": ["events"],
        "parameters": [
          {"$ref": "#/components/parameters/PageSize"},
          {"$ref": "#/components/parameters/Cursor"},
          {"name": "camera_id", "in": "query", "schema": {"type": "string"}},
          {"name": "kind", "in": "query", "schema": {"type": "string"}},
          {"name": "min_severity", "in": "query", "schema": {"type": "string", "enum": ["info", "warning", "critical"]}},
          {"name": "start_time", "in": "query", "schema": {"type": "string", "format": "date-time"}},
          {"name": "end_time", "in": "query", "schema": {"type": "string", "format": "date-time"}}
        ],
        "responses": {
          "200": {
            "description": "Successful response",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "events": {"type": "array", "items": {"$ref": "#/components/schemas/Event"}},
                    "pagination": {"$ref": "#/components/schemas/Pagination"}
                  }
                }
              }
            }
          }
        }
      }
    },
    "/events/{id}": {
      "get": {
        "summary": "Get event",
        "operationId": "getEvent",
        "tags": ["events"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "200": {"description": "Successful response", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Event"}}}}
        }
      },
      "delete": {
        "summary": "Delete event",
        "operationId": "deleteEvent",
        "tags": ["events"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "204": {"description": "Deleted"}
        }
      }
    },
    "/events/{id}/acknowledge": {
      "post": {
        "summary": "Acknowledge event",
        "operationId": "acknowledgeEvent",
        "tags": ["events"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "requestBody": {
          "content": {"application/json": {"schema": {"type": "object", "properties": {"note": {"type": "string"}}}}}
        },
        "responses": {
          "200": {"description": "Acknowledged", "content": {"application/json": {"schema": {"$ref": "#/components/schemas/Event"}}}}
        }
      }
    },`)
}

func writeIntegrationPaths(b *strings.Builder) {
	writeCRUDPaths(b, "integrations", "Integration",
		"Manage third-party integrations")
	b.WriteString(`
    "/integrations/{id}/test": {
      "post": {
        "summary": "Test integration connectivity",
        "operationId": "testIntegration",
        "tags": ["integrations"],
        "parameters": [{"name": "id", "in": "path", "required": true, "schema": {"type": "string"}}],
        "responses": {
          "200": {
            "description": "Test result",
            "content": {
              "application/json": {
                "schema": {
                  "type": "object",
                  "properties": {
                    "success": {"type": "boolean"},
                    "latency_ms": {"type": "integer"},
                    "message": {"type": "string"}
                  }
                }
              }
            }
          }
        }
      }
    },`)
}

func singularize(s string) string {
	if strings.HasSuffix(s, "ies") {
		return s[:len(s)-3] + "y"
	}
	if strings.HasSuffix(s, "ses") {
		return s[:len(s)-2]
	}
	if strings.HasSuffix(s, "s") {
		return s[:len(s)-1]
	}
	return s
}
