package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"cs485/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// JWTClaims are the custom claims embedded in every issued token.
type JWTClaims struct {
	UserID string `json:"userId"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// AuthService handles user registration, login, and JWT issuance/validation.
type AuthService struct {
	db        *sql.DB
	jwtSecret []byte
}

func NewAuthService(db *sql.DB, jwtSecret string) *AuthService {
	return &AuthService{db: db, jwtSecret: []byte(jwtSecret)}
}

// Register creates a new user. Returns an error if the email is already used.
func (s *AuthService) Register(email, displayName, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	id := uuid.New().String()
	now := fmtTime(time.Now())
	_, err = s.db.Exec(
		`INSERT INTO users (id, email, display_name, password_hash, created_at) VALUES ($1, $2, $3, $4, $5)`,
		id, email, displayName, string(hash), now,
	)
	if err != nil {
		return nil, fmt.Errorf("register: %w", err)
	}
	return s.GetByID(id)
}

// Login verifies credentials and returns a signed JWT + user.
func (s *AuthService) Login(email, password string) (string, *models.User, error) {
	user, err := s.getByEmail(email)
	if err != nil {
		return "", nil, errors.New("invalid credentials")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, errors.New("invalid credentials")
	}
	token, err := s.issueToken(user)
	if err != nil {
		return "", nil, err
	}
	return token, user, nil
}

// IssueTokenForUser creates a JWT for an existing user by ID (used by DevBootstrap).
func (s *AuthService) IssueTokenForUser(userID string) (string, *models.User, error) {
	user, err := s.GetByID(userID)
	if err != nil {
		return "", nil, err
	}
	token, err := s.issueToken(user)
	if err != nil {
		return "", nil, err
	}
	return token, user, nil
}

// ValidateToken parses and verifies a JWT, returning its claims.
func (s *AuthService) ValidateToken(tokenStr string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &JWTClaims{}, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return s.jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}

// GetByID retrieves a user by primary key.
func (s *AuthService) GetByID(id string) (*models.User, error) {
	var u models.User
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, email, display_name, password_hash, created_at FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

func (s *AuthService) getByEmail(email string) (*models.User, error) {
	var u models.User
	var createdAt string
	err := s.db.QueryRow(
		`SELECT id, email, display_name, password_hash, created_at FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.DisplayName, &u.PasswordHash, &createdAt)
	if err != nil {
		return nil, err
	}
	u.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &u, nil
}

func (s *AuthService) issueToken(u *models.User) (string, error) {
	claims := &JWTClaims{
		UserID: u.ID,
		Email:  u.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.jwtSecret)
}

func fmtTime(t time.Time) string { return t.UTC().Format(time.RFC3339) }
func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}
