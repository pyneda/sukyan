// Code generated by swaggo/swag. DO NOT EDIT.

package docs

import "github.com/swaggo/swag"

const docTemplate = `{
    "schemes": {{ marshal .Schemes }},
    "swagger": "2.0",
    "info": {
        "description": "{{escape .Description}}",
        "title": "{{.Title}}",
        "contact": {},
        "version": "{{.Version}}"
    },
    "host": "{{.Host}}",
    "basePath": "{{.BasePath}}",
    "paths": {
        "/api/v1/auth/token/renew": {
            "post": {
                "security": [
                    {
                        "ApiKeyAuth": []
                    }
                ],
                "description": "Renew access and refresh tokens.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Auth"
                ],
                "summary": "renew access and refresh tokens",
                "parameters": [
                    {
                        "description": "Refresh token",
                        "name": "refresh_token",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "type": "string"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "ok",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/v1/auth/user/sign/in": {
            "post": {
                "description": "Auth user and return access and refresh token.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Auth"
                ],
                "summary": "auth user and return access and refresh token",
                "parameters": [
                    {
                        "description": "SignIn payload",
                        "name": "signIn",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.SignIn"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.SignInResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/auth/user/sign/out": {
            "post": {
                "security": [
                    {
                        "ApiKeyAuth": []
                    }
                ],
                "description": "De-authorize user and delete refresh token.",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Auth"
                ],
                "summary": "de-authorize user and delete refresh token",
                "responses": {
                    "204": {
                        "description": "ok",
                        "schema": {
                            "type": "string"
                        }
                    }
                }
            }
        },
        "/api/v1/history": {
            "get": {
                "description": "Get history with optional pagination and filtering by status codes, HTTP methods, and sources",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "History"
                ],
                "summary": "Get history",
                "parameters": [
                    {
                        "type": "integer",
                        "default": 50,
                        "description": "Size of each page",
                        "name": "page_size",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "default": 1,
                        "description": "Page number",
                        "name": "page",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma-separated list of status codes to filter by",
                        "name": "status",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma-separated list of HTTP methods to filter by",
                        "name": "methods",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma-separated list of sources to filter by",
                        "name": "sources",
                        "in": "query"
                    }
                ],
                "responses": {
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/history/root-nodes": {
            "get": {
                "description": "Get all the root history items",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "History"
                ],
                "summary": "Gets all root history nodes",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/api.HistorySummary"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/history/websocket/connections": {
            "get": {
                "description": "Get WebSocket connections with optional pagination",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "History WebSocket"
                ],
                "summary": "Get WebSocket connections",
                "parameters": [
                    {
                        "type": "integer",
                        "default": 50,
                        "description": "Size of each page",
                        "name": "page_size",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "default": 1,
                        "description": "Page number",
                        "name": "page",
                        "in": "query"
                    }
                ],
                "responses": {
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/history/websocket/messages": {
            "get": {
                "description": "Get WebSocket messages with optional pagination and filtering by connection id",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "History WebSocket"
                ],
                "summary": "Get WebSocket messages",
                "parameters": [
                    {
                        "type": "integer",
                        "default": 50,
                        "description": "Size of each page",
                        "name": "page_size",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "default": 1,
                        "description": "Page number",
                        "name": "page",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Filter messages by WebSocket connection ID",
                        "name": "connection_id",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/db.WebSocketMessage"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/history/{id}/children": {
            "get": {
                "description": "Get all the other history items that have the same depth or more than the provided history ID and that start with the same URL",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "History"
                ],
                "summary": "Get children history",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "History ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/api.HistorySummary"
                            }
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "404": {
                        "description": "Not Found",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/interactions": {
            "get": {
                "description": "Get interactions with optional pagination and protocols filter",
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Interactions"
                ],
                "summary": "Get interactions",
                "parameters": [
                    {
                        "type": "integer",
                        "default": 50,
                        "description": "Size of each page",
                        "name": "page_size",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "default": 1,
                        "description": "Page number",
                        "name": "page",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma-separated list of protocols to filter by",
                        "name": "protocols",
                        "in": "query"
                    }
                ],
                "responses": {
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/issues": {
            "get": {
                "description": "Retrieves all issues with a count",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Issues"
                ],
                "summary": "List all issues",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/db.Issue"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/issues/grouped": {
            "get": {
                "description": "Retrieves all issues grouped",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Issues"
                ],
                "summary": "List all issues grouped",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/db.GroupedIssue"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/scan/active": {
            "post": {
                "description": "Receives a list of items and schedules them for active scanning",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Scan"
                ],
                "summary": "Submit items for active scanning",
                "parameters": [
                    {
                        "description": "List of items",
                        "name": "input",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.PassiveScanInput"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.ActionResponse"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/scan/passive": {
            "post": {
                "description": "Receives a list of items and schedules them for passive scanning",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Scan"
                ],
                "summary": "Submit items for passive scanning",
                "parameters": [
                    {
                        "description": "List of items",
                        "name": "input",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.PassiveScanInput"
                        }
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "$ref": "#/definitions/api.ActionResponse"
                        }
                    },
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/tasks": {
            "get": {
                "description": "Retrieves tasks based on pagination and status filters",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Tasks"
                ],
                "summary": "List tasks with pagination and filtering",
                "parameters": [
                    {
                        "type": "integer",
                        "default": 50,
                        "description": "Number of items per page",
                        "name": "page_size",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "default": 1,
                        "description": "Page number",
                        "name": "page",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma-separated list of statuses to filter",
                        "name": "status",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/db.Task"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/tasks/jobs": {
            "get": {
                "description": "Allows to filter and search task jobs",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Tasks"
                ],
                "summary": "Search Task Jobs",
                "parameters": [
                    {
                        "type": "integer",
                        "description": "Task ID",
                        "name": "id",
                        "in": "path",
                        "required": true
                    },
                    {
                        "type": "integer",
                        "default": 50,
                        "description": "Number of items per page",
                        "name": "page_size",
                        "in": "query"
                    },
                    {
                        "type": "integer",
                        "default": 1,
                        "description": "Page number",
                        "name": "page",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma-separated list of statuses to filter",
                        "name": "status",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Comma-separated list of titles to filter",
                        "name": "title",
                        "in": "query"
                    },
                    {
                        "type": "string",
                        "description": "Completed at date to filter",
                        "name": "completed_at",
                        "in": "query"
                    }
                ],
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/db.TaskJob"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/tokens/jwts": {
            "post": {
                "description": "Retrieves a list of JWTs with optional filtering and sorting options",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "JWT"
                ],
                "summary": "List JWTs with filtering and sorting",
                "parameters": [
                    {
                        "description": "Filtering and sorting options",
                        "name": "input",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/db.JwtFilters"
                        }
                    }
                ],
                "responses": {
                    "400": {
                        "description": "Bad Request",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        },
        "/api/v1/workspaces": {
            "get": {
                "description": "Retrieves all workspaces with a count",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Workspaces"
                ],
                "summary": "List all workspaces",
                "responses": {
                    "200": {
                        "description": "OK",
                        "schema": {
                            "type": "array",
                            "items": {
                                "$ref": "#/definitions/db.Workspace"
                            }
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            },
            "post": {
                "description": "Saves a new workspace to the database",
                "consumes": [
                    "application/json"
                ],
                "produces": [
                    "application/json"
                ],
                "tags": [
                    "Workspaces"
                ],
                "summary": "Create a new workspace",
                "parameters": [
                    {
                        "description": "Workspace to create",
                        "name": "workspace",
                        "in": "body",
                        "required": true,
                        "schema": {
                            "$ref": "#/definitions/api.WorkspaceCreateInput"
                        }
                    }
                ],
                "responses": {
                    "201": {
                        "description": "Created",
                        "schema": {
                            "$ref": "#/definitions/db.Workspace"
                        }
                    },
                    "500": {
                        "description": "Internal Server Error",
                        "schema": {
                            "$ref": "#/definitions/api.ErrorResponse"
                        }
                    }
                }
            }
        }
    },
    "definitions": {
        "api.ActionResponse": {
            "type": "object",
            "properties": {
                "message": {
                    "type": "string"
                }
            }
        },
        "api.ErrorResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "type": "string"
                },
                "message": {
                    "type": "string"
                }
            }
        },
        "api.HistorySummary": {
            "type": "object",
            "properties": {
                "depth": {
                    "type": "integer"
                },
                "id": {
                    "type": "integer"
                },
                "method": {
                    "type": "string"
                },
                "parameters_count": {
                    "type": "integer"
                },
                "status_code": {
                    "type": "integer"
                },
                "url": {
                    "type": "string"
                }
            }
        },
        "api.PassiveScanInput": {
            "type": "object",
            "required": [
                "items"
            ],
            "properties": {
                "items": {
                    "type": "array",
                    "items": {
                        "type": "integer"
                    }
                }
            }
        },
        "api.SignIn": {
            "type": "object",
            "required": [
                "email",
                "password"
            ],
            "properties": {
                "email": {
                    "type": "string",
                    "maxLength": 255
                },
                "password": {
                    "type": "string",
                    "maxLength": 255
                }
            }
        },
        "api.SignInResponse": {
            "type": "object",
            "properties": {
                "error": {
                    "type": "boolean"
                },
                "msg": {
                    "type": "string"
                },
                "tokens": {
                    "$ref": "#/definitions/api.SignInTokens"
                }
            }
        },
        "api.SignInTokens": {
            "type": "object",
            "properties": {
                "access": {
                    "type": "string"
                },
                "refresh": {
                    "type": "string"
                }
            }
        },
        "api.WorkspaceCreateInput": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string"
                },
                "description": {
                    "type": "string"
                },
                "title": {
                    "type": "string"
                }
            }
        },
        "db.GroupedIssue": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string"
                },
                "count": {
                    "type": "integer"
                },
                "severity": {
                    "type": "string"
                },
                "title": {
                    "type": "string"
                }
            }
        },
        "db.Issue": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string"
                },
                "confidence": {
                    "type": "integer"
                },
                "created_at": {
                    "type": "string"
                },
                "curl_command": {
                    "description": "Severity string ` + "`" + `json:\"severity\" gorm:\"type:ENUM('Unknown', 'Info', 'Low', 'Medium', 'High', 'Critical');default:'Info'\"` + "`" + `\nSeverity    string ` + "`" + `json:\"severity\" gorm:\"index; default:'Unknown'\"` + "`" + `",
                    "type": "string"
                },
                "cwe": {
                    "type": "integer"
                },
                "description": {
                    "type": "string"
                },
                "details": {
                    "type": "string"
                },
                "false_positive": {
                    "type": "boolean"
                },
                "http_method": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                },
                "note": {
                    "type": "string"
                },
                "payload": {
                    "type": "string"
                },
                "references": {
                    "type": "array",
                    "items": {
                        "type": "string"
                    }
                },
                "remediation": {
                    "type": "string"
                },
                "request": {
                    "type": "array",
                    "items": {
                        "type": "integer"
                    }
                },
                "response": {
                    "type": "array",
                    "items": {
                        "type": "integer"
                    }
                },
                "severity": {
                    "description": "enums seem to fail - review later",
                    "allOf": [
                        {
                            "$ref": "#/definitions/db.severity"
                        }
                    ]
                },
                "status_code": {
                    "type": "integer"
                },
                "title": {
                    "type": "string"
                },
                "updated_at": {
                    "type": "string"
                },
                "url": {
                    "type": "string"
                }
            }
        },
        "db.JwtFilters": {
            "type": "object",
            "properties": {
                "algorithm": {
                    "type": "string",
                    "enum": [
                        "HS256",
                        "HS384",
                        "HS512",
                        "RS256",
                        "RS384",
                        "RS512",
                        "ES256",
                        "ES384",
                        "ES512"
                    ]
                },
                "audience": {
                    "type": "string"
                },
                "issuer": {
                    "type": "string"
                },
                "sort_by": {
                    "description": "Example validation rule for sort_by",
                    "type": "string",
                    "enum": [
                        "token",
                        "header",
                        "issuer",
                        "id",
                        "algorithm",
                        "subject",
                        "audience",
                        "expiration",
                        "issued_at"
                    ]
                },
                "sort_order": {
                    "description": "Example validation rule for sort_order",
                    "type": "string",
                    "enum": [
                        "asc",
                        "desc"
                    ]
                },
                "subject": {
                    "type": "string"
                }
            }
        },
        "db.MessageDirection": {
            "type": "string",
            "enum": [
                "sent",
                "received"
            ],
            "x-enum-varnames": [
                "MessageSent",
                "MessageReceived"
            ]
        },
        "db.Task": {
            "type": "object",
            "properties": {
                "created_at": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                },
                "started_at": {
                    "type": "string"
                },
                "status": {
                    "type": "string"
                },
                "updated_at": {
                    "type": "string"
                }
            }
        },
        "db.TaskJob": {
            "type": "object",
            "properties": {
                "completed_at": {
                    "type": "string"
                },
                "created_at": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                },
                "status": {
                    "type": "string"
                },
                "task_id": {
                    "type": "integer"
                },
                "title": {
                    "type": "string"
                },
                "updated_at": {
                    "type": "string"
                }
            }
        },
        "db.WebSocketMessage": {
            "type": "object",
            "properties": {
                "connection_id": {
                    "type": "integer"
                },
                "created_at": {
                    "type": "string"
                },
                "direction": {
                    "description": "direction of the message",
                    "allOf": [
                        {
                            "$ref": "#/definitions/db.MessageDirection"
                        }
                    ]
                },
                "id": {
                    "type": "integer"
                },
                "mask": {
                    "type": "boolean"
                },
                "opcode": {
                    "type": "number"
                },
                "payload_data": {
                    "type": "string"
                },
                "timestamp": {
                    "description": "timestamp for when the message was sent/received",
                    "type": "string"
                },
                "updated_at": {
                    "type": "string"
                }
            }
        },
        "db.Workspace": {
            "type": "object",
            "properties": {
                "code": {
                    "type": "string"
                },
                "created_at": {
                    "type": "string"
                },
                "description": {
                    "type": "string"
                },
                "id": {
                    "type": "integer"
                },
                "title": {
                    "type": "string"
                },
                "updated_at": {
                    "type": "string"
                }
            }
        },
        "db.severity": {
            "type": "string",
            "enum": [
                "Unknown",
                "Info",
                "Low",
                "Medium",
                "High",
                "Critical"
            ],
            "x-enum-varnames": [
                "Unknown",
                "Info",
                "Low",
                "Medium",
                "High",
                "Critical"
            ]
        }
    }
}`

// SwaggerInfo holds exported Swagger Info so clients can modify it
var SwaggerInfo = &swag.Spec{
	Version:          "",
	Host:             "",
	BasePath:         "",
	Schemes:          []string{},
	Title:            "",
	Description:      "",
	InfoInstanceName: "swagger",
	SwaggerTemplate:  docTemplate,
	LeftDelim:        "{{",
	RightDelim:       "}}",
}

func init() {
	swag.Register(SwaggerInfo.InstanceName(), SwaggerInfo)
}
