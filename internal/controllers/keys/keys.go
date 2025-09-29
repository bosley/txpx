package keys

import (
	"errors"
	"log/slog"
	"time"

	paseto "aidanwoods.dev/go-paseto"
	"github.com/bosley/txpx/internal/controllers/users"
	"github.com/bosley/txpx/internal/models"
	"github.com/google/uuid"
)

var (
	ErrKeyNotFound         = errors.New("key not found")
	ErrKeyNameInUseForUser = errors.New("key name already in use for user")
	ErrApiKeyNotFound      = errors.New("api key not found")
)

type Keys interface {
	CreateApiKeyForUser(userUUID string, name string) (string, error)
	ListAllApiKeysForUser(userUUID string) ([]string, error) // only returns key names, not values
	DeleteApiKeyForUser(userUUID string, name string) error
	GetApiKeyForUser(userUUID string, name string) (string, error)
	UpdateApiKeyForUser(userUUID string, name string, newName string) error
	GetUserFromApiKey(apiKey string) (*models.User, error)
}

func New(
	logger *slog.Logger,
	userController users.Users,
) Keys {
	return &keysImpl{
		logger:         logger,
		userController: userController,

		issuedAPiKeystringToUserUUID: make(map[string]string),
		mockApiKeys:                  make(map[string]map[string]*models.ApiKey),
		mockApiKeyTokens:             make(map[string]string),
	}
}

type keysImpl struct {
	logger         *slog.Logger
	userController users.Users

	issuedAPiKeystringToUserUUID map[string]string
	mockApiKeys                  map[string]map[string]*models.ApiKey
	mockApiKeyTokens             map[string]string
}

func (x *keysImpl) CreateApiKeyForUser(userUUID string, name string) (string, error) {
	_, err := x.userController.GetUserByUUID(userUUID)
	if err != nil {
		return "", err
	}

	if _, exists := x.mockApiKeys[userUUID]; !exists {
		x.mockApiKeys[userUUID] = make(map[string]*models.ApiKey)
	}

	if _, exists := x.mockApiKeys[userUUID][name]; exists {
		return "", ErrKeyNameInUseForUser
	}

	apiKey := newApiKeyModel(userUUID, name)

	secretKey, err := paseto.NewV4AsymmetricSecretKeyFromHex(apiKey.SecretKey)
	if err != nil {
		return "", err
	}

	token := paseto.NewToken()
	token.SetIssuedAt(apiKey.CreatedAt)
	token.SetExpiration(apiKey.CreatedAt.Add(time.Hour * 24 * 30))
	token.SetIssuer("txpx")
	token.SetSubject("txpx-api-key")
	token.SetAudience("txpx-users")
	token.SetJti(apiKey.UUID)
	token.SetString("user_uuid", userUUID)
	token.SetNotBefore(apiKey.CreatedAt)

	tokenString := token.V4Sign(secretKey, nil)

	x.mockApiKeys[userUUID][name] = apiKey
	x.mockApiKeyTokens[apiKey.UUID] = tokenString
	x.issuedAPiKeystringToUserUUID[tokenString] = userUUID

	return tokenString, nil
}

func (x *keysImpl) ListAllApiKeysForUser(userUUID string) ([]string, error) {
	_, err := x.userController.GetUserByUUID(userUUID)
	if err != nil {
		return nil, err
	}

	if _, exists := x.mockApiKeys[userUUID]; !exists {
		return []string{}, nil
	}

	names := make([]string, 0, len(x.mockApiKeys[userUUID]))
	for name := range x.mockApiKeys[userUUID] {
		names = append(names, name)
	}

	return names, nil
}

func (x *keysImpl) DeleteApiKeyForUser(userUUID string, name string) error {
	_, err := x.userController.GetUserByUUID(userUUID)
	if err != nil {
		return err
	}

	if _, exists := x.mockApiKeys[userUUID]; !exists {
		return ErrKeyNotFound
	}

	apiKey, exists := x.mockApiKeys[userUUID][name]
	if !exists {
		return ErrKeyNotFound
	}

	tokenString := x.mockApiKeyTokens[apiKey.UUID]
	delete(x.mockApiKeyTokens, apiKey.UUID)
	delete(x.issuedAPiKeystringToUserUUID, tokenString)
	delete(x.mockApiKeys[userUUID], name)

	return nil
}

func (x *keysImpl) GetApiKeyForUser(userUUID string, name string) (string, error) {
	_, err := x.userController.GetUserByUUID(userUUID)
	if err != nil {
		return "", err
	}

	if _, exists := x.mockApiKeys[userUUID]; !exists {
		return "", ErrKeyNotFound
	}

	apiKey, exists := x.mockApiKeys[userUUID][name]
	if !exists {
		return "", ErrKeyNotFound
	}

	tokenString := x.mockApiKeyTokens[apiKey.UUID]
	return tokenString, nil
}

func (x *keysImpl) UpdateApiKeyForUser(userUUID string, name string, newName string) error {
	_, err := x.userController.GetUserByUUID(userUUID)
	if err != nil {
		return err
	}

	if _, exists := x.mockApiKeys[userUUID]; !exists {
		return ErrKeyNotFound
	}

	apiKey, exists := x.mockApiKeys[userUUID][name]
	if !exists {
		return ErrKeyNotFound
	}

	if _, exists := x.mockApiKeys[userUUID][newName]; exists {
		return ErrKeyNameInUseForUser
	}

	apiKey.Name = newName
	apiKey.UpdatedAt = time.Now()

	delete(x.mockApiKeys[userUUID], name)
	x.mockApiKeys[userUUID][newName] = apiKey

	return nil
}

func (x *keysImpl) GetUserFromApiKey(apiKey string) (*models.User, error) {
	userUUID, exists := x.issuedAPiKeystringToUserUUID[apiKey]
	if !exists {
		return nil, ErrApiKeyNotFound
	}

	var foundApiKey *models.ApiKey
	for _, apiKeys := range x.mockApiKeys[userUUID] {
		if x.mockApiKeyTokens[apiKeys.UUID] == apiKey {
			foundApiKey = apiKeys
			break
		}
	}

	if foundApiKey == nil {
		return nil, ErrApiKeyNotFound
	}

	publicKey, err := paseto.NewV4AsymmetricPublicKeyFromHex(foundApiKey.PublicKey)
	if err != nil {
		return nil, err
	}

	parser := paseto.NewParser()
	parser.AddRule(paseto.IssuedBy("txpx"))
	parser.AddRule(paseto.Subject("txpx-api-key"))
	parser.AddRule(paseto.ForAudience("txpx-users"))
	parser.AddRule(paseto.IdentifiedBy(foundApiKey.UUID))
	parser.AddRule(paseto.NotExpired())

	token, err := parser.ParseV4Public(publicKey, apiKey, nil)
	if err != nil {
		return nil, err
	}

	extractedUserUUID, err := token.GetString("user_uuid")
	if err != nil {
		return nil, err
	}

	user, err := x.userController.GetUserByUUID(extractedUserUUID)
	if err != nil {
		return nil, err
	}

	return user, nil
}

func newApiKeyModel(userUUID string, name string) *models.ApiKey {
	mod := &models.ApiKey{
		UUID:      uuid.New().String(),
		UserUUID:  userUUID,
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	token := paseto.NewToken()
	token.SetIssuedAt(time.Now())
	token.SetExpiration(time.Now().Add(time.Hour * 24 * 30))
	token.SetIssuer("txpx")
	token.SetSubject("txpx-api-key")
	token.SetAudience("txpx-users")
	token.SetJti(mod.UUID)
	token.SetString("user_uuid", userUUID)
	token.SetNotBefore(time.Now())

	secretKey := paseto.NewV4AsymmetricSecretKey()
	publicKey := secretKey.Public()

	mod.SecretKey = secretKey.ExportHex()
	mod.PublicKey = publicKey.ExportHex()

	return mod
}
