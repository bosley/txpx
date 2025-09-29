package models

import "time"

type ApiKey struct {
	UUID      string    `json:"uuid"`
	UserUUID  string    `json:"user_uuid"`
	Name      string    `json:"name"`       // Display name for the api key
	PublicKey string    `json:"public_key"` // PASETO public key
	SecretKey string    `json:"secret_key"` // PASETO secret key
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
