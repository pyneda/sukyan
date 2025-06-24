package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
)

type JsonWebToken struct {
	BaseModel
	Token                  string                `gorm:"type:text" json:"token"`
	Header                 datatypes.JSON        `gorm:"type:json" json:"header" swaggerignore:"true"`
	Payload                datatypes.JSON        `gorm:"type:json" json:"payload" swaggerignore:"true"`
	Signature              string                `gorm:"type:text" json:"signature"`
	Algorithm              string                `gorm:"type:text" json:"algorithm"`
	Issuer                 string                `gorm:"type:text" json:"issuer"`
	Subject                string                `gorm:"type:text" json:"subject"`
	Audience               string                `gorm:"type:text" json:"audience"`
	Expiration             time.Time             `gorm:"type:timestamp" json:"expiration"`
	IssuedAt               time.Time             `gorm:"type:timestamp" json:"issued_at"`
	Histories              []History             `gorm:"many2many:json_web_token_histories;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"histories"`
	Workspace              Workspace             `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID            *uint                 `json:"workspace_id"`
	TestedEmbeddedWordlist bool                  `json:"tested_embedded_wordlist"`
	Cracked                bool                  `json:"cracked"`
	Secret                 string                `json:"secret"`
	WebSocketConnections   []WebSocketConnection `gorm:"many2many:json_web_token_websocket_connections;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"websocket_connections"`
}

func (j JsonWebToken) TableHeaders() []string {
	return []string{"ID", "Token", "Algorithm", "Issuer", "Subject", "Audience", "Expiration", "IssuedAt", "WorkspaceID"}
}

func (j JsonWebToken) TableRow() []string {
	return []string{
		fmt.Sprintf("%d", j.ID),
		j.Token,
		j.Algorithm,
		j.Issuer,
		j.Subject,
		j.Audience,
		j.Expiration.Format(time.RFC3339),
		j.IssuedAt.Format(time.RFC3339),
		fmt.Sprintf("%d", *j.WorkspaceID),
	}
}

func (j JsonWebToken) String() string {
	return fmt.Sprintf("ID: %d, Token: %s, Algorithm: %s, Issuer: %s, Subject: %s, Audience: %s, Expiration: %s, IssuedAt: %s, WorkspaceID: %d", j.ID, j.Token, j.Algorithm, j.Issuer, j.Subject, j.Audience, j.Expiration.Format(time.RFC3339), j.IssuedAt.Format(time.RFC3339), j.WorkspaceID)
}

func (j JsonWebToken) Pretty() string {
	return fmt.Sprintf(
		"%sID:%s %d\n%sToken:%s %s\n%sAlgorithm:%s %s\n%sIssuer:%s %s\n%sSubject:%s %s\n%sAudience:%s %s\n%sExpiration:%s %s\n%sIssuedAt:%s %s\n%sWorkspaceID:%s %d\n",
		lib.Blue, lib.ResetColor, j.ID,
		lib.Blue, lib.ResetColor, j.Token,
		lib.Blue, lib.ResetColor, j.Algorithm,
		lib.Blue, lib.ResetColor, j.Issuer,
		lib.Blue, lib.ResetColor, j.Subject,
		lib.Blue, lib.ResetColor, j.Audience,
		lib.Blue, lib.ResetColor, j.Expiration.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, j.IssuedAt.Format(time.RFC3339),
		lib.Blue, lib.ResetColor, j.WorkspaceID)
}

// FillJwtFromToken fills a JsonWebToken struct with data extracted from the given JWT token.
func FillJwtFromToken(jwtToken string) (*JsonWebToken, error) {
	parts := strings.Split(jwtToken, ".")
	if len(parts) != 3 {
		return nil, errors.New("invalid JWT format")
	}

	headerJSON, err := lib.Base64Decode(parts[0])
	if err != nil {
		return nil, err
	}

	claimsJSON, err := lib.Base64Decode(parts[1])
	if err != nil {
		return nil, err
	}

	signature := parts[2]

	// Extract the algorithm from the header
	var header struct {
		Algorithm string `json:"alg"`
	}
	if err := json.Unmarshal(headerJSON, &header); err != nil {
		return nil, errors.New("invalid JWT header")
	}

	// Extract the standard claims from the claims JSON
	var claims struct {
		Subject   string `json:"sub"`
		Issuer    string `json:"iss"`
		Audience  string `json:"aud"`
		ExpiresAt int64  `json:"exp"`
		IssuedAt  int64  `json:"iat"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		return nil, errors.New("invalid JWT claims")
	}

	// Create the JsonWebToken instance
	jwtInstance := &JsonWebToken{
		Signature:  signature,
		Header:     datatypes.JSON(headerJSON),
		Payload:    datatypes.JSON(claimsJSON),
		Token:      jwtToken,
		Algorithm:  header.Algorithm,
		Issuer:     claims.Issuer,
		Subject:    claims.Subject,
		Audience:   claims.Audience,
		Expiration: time.Unix(claims.ExpiresAt, 0),
		IssuedAt:   time.Unix(claims.IssuedAt, 0),
	}

	return jwtInstance, nil
}

// GetOrCreateJWTFromTokenAndHistory checks if JWT with the same signature already exists in the DB
func (d *DatabaseConnection) GetOrCreateJWTFromTokenAndHistory(jwtToken string, historyID uint) (*JsonWebToken, error) {
	jwtInstance, err := FillJwtFromToken(jwtToken)
	if err != nil {
		log.Error().Err(err).Str("token", jwtToken).Msg("Failed to fill JWT from token")
		return nil, err
	}

	// Check if JWT with the same signature already exists in the DB
	// If it doesn't, create a new record
	// If it does, fetch that record
	d.db.FirstOrCreate(&jwtInstance, JsonWebToken{Signature: jwtInstance.Signature})
	log.Warn().Interface("jwt", jwtInstance).Msg("JWT and history relation")

	// Add relation to History
	history := &History{}
	d.db.First(&history, historyID)
	if history != nil {
		d.db.Model(&history).Association("JsonWebTokens").Append(jwtInstance)
	}

	return jwtInstance, nil
}

// GetOrCreateJWTFromTokenAndWebSocketMessage checks if JWT with the same signature already exists in the DB
// If it doesn't, create a new record. If it does, fetch that record.
func (d *DatabaseConnection) GetOrCreateJWTFromTokenAndWebSocketMessage(jwtToken string, messageID uint) (*JsonWebToken, error) {
	jwtInstance, err := FillJwtFromToken(jwtToken)
	if err != nil {
		log.Error().Err(err).Str("token", jwtToken).Msg("Failed to fill JWT from token")
		return nil, err
	}

	d.db.FirstOrCreate(&jwtInstance, JsonWebToken{Signature: jwtInstance.Signature})
	log.Info().Interface("jwt", jwtInstance).Uint("message_id", messageID).Msg("JWT found in WebSocket message")

	message := &WebSocketMessage{}
	if err := d.db.First(&message, messageID).Error; err != nil {
		log.Error().Err(err).Uint("message_id", messageID).Msg("Failed to find WebSocket message")
		return jwtInstance, nil
	}

	// Get the WebSocket connection and establish the relationship
	connection := &WebSocketConnection{}
	if err := d.db.First(&connection, message.ConnectionID).Error; err == nil {
		if connection.WorkspaceID != nil {
			jwtInstance.WorkspaceID = connection.WorkspaceID
			d.db.Save(&jwtInstance)
		}

		d.db.Model(&connection).Association("JsonWebTokens").Append(jwtInstance)
		log.Info().Uint("connection_id", connection.ID).Uint("jwt_id", jwtInstance.ID).Msg("Associated JWT with WebSocket connection")
	}

	return jwtInstance, nil
}

// GetOrCreateJWTFromTokenAndWebSocketConnection checks if JWT with the same signature already exists in the DB
func (d *DatabaseConnection) GetOrCreateJWTFromTokenAndWebSocketConnection(jwtToken string, connectionID uint) (*JsonWebToken, error) {
	jwtInstance, err := FillJwtFromToken(jwtToken)
	if err != nil {
		log.Error().Err(err).Str("token", jwtToken).Msg("Failed to fill JWT from token")
		return nil, err
	}

	d.db.FirstOrCreate(&jwtInstance, JsonWebToken{Signature: jwtInstance.Signature})
	log.Info().Interface("jwt", jwtInstance).Uint("connection_id", connectionID).Msg("JWT found in WebSocket connection headers")

	// Get the WebSocket connection and establish the relationship
	connection := &WebSocketConnection{}
	if err := d.db.First(&connection, connectionID).Error; err == nil {
		if connection.WorkspaceID != nil {
			jwtInstance.WorkspaceID = connection.WorkspaceID
			d.db.Save(&jwtInstance)
		}

		d.db.Model(&connection).Association("JsonWebTokens").Append(jwtInstance)
		log.Info().Uint("connection_id", connection.ID).Uint("jwt_id", jwtInstance.ID).Msg("Associated JWT with WebSocket connection")
	} else {
		log.Error().Err(err).Uint("connection_id", connectionID).Msg("Failed to find WebSocket connection")
	}

	return jwtInstance, nil
}

type JwtFilters struct {
	Token       string `json:"token" validate:"omitempty"`
	Algorithm   string `json:"algorithm" validate:"omitempty,oneof=HS256 HS384 HS512 RS256 RS384 RS512 ES256 ES384 ES512"`
	Issuer      string `json:"issuer"`
	Subject     string `json:"subject"`
	Audience    string `json:"audience"`
	SortBy      string `json:"sort_by" validate:"omitempty,oneof=token header issuer id algorithm subject audience expiration issued_at"` // Example validation rule for sort_by
	SortOrder   string `json:"sort_order" validate:"omitempty,oneof=asc desc"`                                                            // Example validation rule for sort_order
	WorkspaceID uint   `json:"workspace_id" validate:"omitempty,numeric"`
}

func (d *DatabaseConnection) ListJsonWebTokens(filters JwtFilters) ([]*JsonWebToken, error) {
	query := d.db.Model(&JsonWebToken{})

	// Add filtering conditions based on the input values
	if filters.Algorithm != "" {
		query = query.Where("algorithm = ?", filters.Algorithm)
	}
	if filters.Issuer != "" {
		query = query.Where("issuer = ?", filters.Issuer)
	}
	if filters.Subject != "" {
		query = query.Where("subject = ?", filters.Subject)
	}
	if filters.Audience != "" {
		query = query.Where("audience = ?", filters.Audience)
	}

	if filters.Token != "" {
		query = query.Where("token LIKE ?", fmt.Sprintf("%%%s%%", filters.Token))
	}

	if filters.WorkspaceID != 0 {
		query = query.Joins("JOIN json_web_token_histories ON json_web_tokens.id = json_web_token_histories.json_web_token_id")
		query = query.Joins("JOIN histories ON json_web_token_histories.history_id = histories.id")
		query = query.Where("histories.workspace_id = ?", filters.WorkspaceID)
	}

	// Define the sorting column and order based on the input values
	sortColumn := "created_at" // Default sorting column
	if filters.SortBy != "" {
		sortColumn = filters.SortBy
	}
	sortOrder := "asc" // Default sorting order
	if filters.SortOrder != "" {
		sortOrder = filters.SortOrder
	}
	query = query.Order(fmt.Sprintf("%s %s", sortColumn, sortOrder))

	// Execute the query and retrieve the filtered and sorted JWTs
	var jwts []*JsonWebToken
	err := query.Find(&jwts).Error

	if err != nil {
		return nil, err
	}
	return jwts, nil
}

func (d *DatabaseConnection) UpdateJWT(jwtID uint, jwt *JsonWebToken) error {
	result := d.db.Model(&JsonWebToken{}).Where("id = ?", jwtID).Updates(jwt)
	if result.Error != nil {
		log.Error().Err(result.Error).Interface("jwt", jwt).Msg("Failed to update JWT")
		return result.Error
	}
	return nil
}
