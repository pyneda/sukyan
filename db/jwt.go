package db

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
	"gorm.io/datatypes"
	"strings"
	"time"
)

type JsonWebToken struct {
	BaseModel
	Token       string         `gorm:"type:text" json:"token"`
	Header      datatypes.JSON `gorm:"type:json" json:"header"`
	Payload     datatypes.JSON `gorm:"type:json" json:"payload"`
	Signature   string         `gorm:"type:text" json:"signature"`
	Algorithm   string         `gorm:"type:text" json:"algorithm"`
	Issuer      string         `gorm:"type:text" json:"issuer"`
	Subject     string         `gorm:"type:text" json:"subject"`
	Audience    string         `gorm:"type:text" json:"audience"`
	Expiration  time.Time      `gorm:"type:timestamp" json:"expiration"`
	IssuedAt    time.Time      `gorm:"type:timestamp" json:"issued_at"`
	Histories   []History      `gorm:"many2many:json_web_token_histories" json:"histories"`
	Workspace   Workspace      `json:"-" gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	WorkspaceID *uint          `json:"workspace_id"`
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

type JwtFilters struct {
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
