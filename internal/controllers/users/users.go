package users

import (
	"errors"
	"time"

	"github.com/bosley/txpx/internal/models"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound = errors.New("user not found")
	ErrEmailInUse   = errors.New("email already in use")
)

type Users interface {
	CreateUser(
		email string,
		password string,
	) (*models.User, error)

	GetUserByEmail(
		email string,
	) (*models.User, error)

	GetUserByUUID(
		userUUID string,
	) (*models.User, error)

	ListAllUsers() ([]*models.User, error)

	UpdateUserName(
		userUUID string,
		name string,
	) (*models.User, error)

	UpdateUserEmail(
		userUUID string,
		email string,
	) (*models.User, error)

	UpdateUserPassword(
		userUUID string,
		password string, // not hashed or filtered, controller responsibility
	) (*models.User, error)

	DeleteUser(
		userUUID string,
	) error
}

type usersImpl struct {
	mockUsers map[string]*models.User
	mockEmail map[string]string
}

var _ Users = &usersImpl{}

func New() Users {
	return &usersImpl{
		mockUsers: make(map[string]*models.User),
		mockEmail: make(map[string]string),
	}
}

func HashUserPassword(password string) (string, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword(
		[]byte(password), bcrypt.DefaultCost,
	)
	if err != nil {
		return "", err
	}
	passwordHashStr := string(hashedPassword)
	return passwordHashStr, nil
}

func VerifyUserPassword(password string, hashedPassword string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password)) == nil
}

func (x *usersImpl) CreateUser(email string, password string) (*models.User, error) {
	if _, exists := x.mockEmail[email]; exists {
		return nil, ErrEmailInUse
	}

	hashedPassword, err := HashUserPassword(password)
	if err != nil {
		return nil, err
	}

	user := &models.User{
		UUID:      uuid.New().String(),
		Email:     email,
		Password:  hashedPassword,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	x.mockUsers[user.UUID] = user
	x.mockEmail[email] = user.UUID
	return user, nil
}

func (x *usersImpl) GetUserByEmail(email string) (*models.User, error) {
	if uuid, exists := x.mockEmail[email]; exists {
		return x.mockUsers[uuid], nil
	}
	return nil, ErrUserNotFound
}

func (x *usersImpl) GetUserByUUID(userUUID string) (*models.User, error) {
	if user, exists := x.mockUsers[userUUID]; exists {
		return user, nil
	}
	return nil, ErrUserNotFound
}

func (x *usersImpl) ListAllUsers() ([]*models.User, error) {
	users := make([]*models.User, 0, len(x.mockUsers))
	for _, user := range x.mockUsers {
		users = append(users, user)
	}
	return users, nil
}

func (x *usersImpl) UpdateUserName(userUUID string, name string) (*models.User, error) {
	if user, exists := x.mockUsers[userUUID]; exists {
		user.Name = name
		user.UpdatedAt = time.Now()
		return user, nil
	}
	return nil, ErrUserNotFound
}

func (x *usersImpl) UpdateUserEmail(userUUID string, email string) (*models.User, error) {
	if _, exists := x.mockEmail[email]; exists {
		return nil, ErrEmailInUse
	}

	if user, exists := x.mockUsers[userUUID]; exists {
		delete(x.mockEmail, user.Email)
		user.Email = email
		user.UpdatedAt = time.Now()
		x.mockEmail[email] = userUUID
		return user, nil
	}
	return nil, ErrUserNotFound
}

func (x *usersImpl) UpdateUserPassword(userUUID string, password string) (*models.User, error) {
	if user, exists := x.mockUsers[userUUID]; exists {
		hashedPassword, err := HashUserPassword(password)
		if err != nil {
			return nil, err
		}
		user.Password = hashedPassword
		user.UpdatedAt = time.Now()
		return user, nil
	}
	return nil, ErrUserNotFound
}

func (x *usersImpl) DeleteUser(userUUID string) error {
	if user, exists := x.mockUsers[userUUID]; exists {
		delete(x.mockEmail, user.Email)
		delete(x.mockUsers, userUUID)
		return nil
	}
	return ErrUserNotFound
}
