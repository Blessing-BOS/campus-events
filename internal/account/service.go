package account

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"campus-events/internal/apierror"
)

type User struct {
	ID           uint64    `json:"id" gorm:"primaryKey"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	CreatedAt    time.Time `json:"created_at"`
}
type Session struct {
	ID        string `gorm:"primaryKey"`
	UserID    uint64
	ExpiresAt time.Time
	CreatedAt time.Time
}

func (Session) TableName() string { return "auth_sessions" }

type Principal struct {
	User      User
	SessionID string
}
type LoginResult struct {
	AccessToken string    `json:"access_token"`
	ExpiresAt   time.Time `json:"expires_at"`
	User        User      `json:"user"`
}
type Service struct {
	db        *gorm.DB
	secret    []byte
	now       func() time.Time
	dummyHash []byte
}

func NewService(db *gorm.DB, secret string, now func() time.Time) *Service {
	dummy, _ := bcrypt.GenerateFromPassword([]byte("dummy-password-no-account"), bcrypt.DefaultCost)
	return &Service{db: db, secret: []byte(secret), now: now, dummyHash: dummy}
}
func NormalizeUsername(name string) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if len(name) < 3 || len(name) > 32 {
		return "", false
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_') {
			return "", false
		}
	}
	return name, true
}
func validPassword(password string) bool {
	return utf8.ValidString(password) && len(password) >= 8 && len(password) <= 72
}
func (s *Service) Register(ctx context.Context, username, password string) (User, error) {
	name, ok := NormalizeUsername(username)
	if !ok || !validPassword(password) {
		return User{}, apierror.New(400, "INVALID_ARGUMENT", "用户名需为 3～32 位字母、数字或下划线；密码需为 8～72 字节")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	u := User{Username: name, PasswordHash: string(hash), Role: "student", CreatedAt: s.now().UTC()}
	if err := s.db.WithContext(ctx).Create(&u).Error; err != nil {
		var duplicate *mysql.MySQLError
		if errors.As(err, &duplicate) && duplicate.Number == 1062 {
			return User{}, apierror.New(409, "USERNAME_TAKEN", "用户名已被使用")
		}
		return User{}, err
	}
	return u, nil
}
func (s *Service) Login(ctx context.Context, username, password string) (LoginResult, error) {
	name, ok := NormalizeUsername(username)
	if !ok || !validPassword(password) {
		return LoginResult{}, apierror.New(401, "INVALID_CREDENTIALS", "用户名或密码不正确")
	}
	var u User
	err := s.db.WithContext(ctx).Where("username = ?", name).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return LoginResult{}, apierror.New(401, "INVALID_CREDENTIALS", "用户名或密码不正确")
	}
	if err != nil {
		return LoginResult{}, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return LoginResult{}, apierror.New(401, "INVALID_CREDENTIALS", "用户名或密码不正确")
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return LoginResult{}, err
	}
	now := s.now().UTC().Truncate(time.Second)
	expires := now.Add(2 * time.Hour)
	session := Session{ID: hex.EncodeToString(id), UserID: u.ID, ExpiresAt: expires, CreatedAt: now}
	claims := jwt.RegisteredClaims{Issuer: "campus-events", Subject: strconv.FormatUint(u.ID, 10), ID: session.ID, IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expires)}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(s.secret)
	if err != nil {
		return LoginResult{}, err
	}
	if err := s.db.WithContext(ctx).Create(&session).Error; err != nil {
		return LoginResult{}, err
	}
	// Expired sessions do not authenticate even if this best-effort cleanup fails.
	_ = s.db.WithContext(ctx).Where("user_id = ? AND expires_at <= ?", u.ID, now).Delete(&Session{}).Error
	return LoginResult{token, expires, u}, nil
}
func (s *Service) Authenticate(ctx context.Context, raw string) (Principal, error) {
	claims := &jwt.RegisteredClaims{}
	_, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (any, error) { return s.secret, nil }, jwt.WithValidMethods([]string{"HS256"}), jwt.WithIssuer("campus-events"), jwt.WithExpirationRequired(), jwt.WithIssuedAt(), jwt.WithTimeFunc(s.now))
	if err != nil || claims.ID == "" {
		return Principal{}, apierror.Unauthorized
	}
	id, err := strconv.ParseUint(claims.Subject, 10, 64)
	if err != nil || id == 0 {
		return Principal{}, apierror.Unauthorized
	}
	var u User
	err = s.db.WithContext(ctx).Model(&User{}).Select("users.*").Joins("JOIN auth_sessions ON auth_sessions.user_id = users.id").Where("users.id = ? AND auth_sessions.id = ? AND auth_sessions.expires_at > ?", id, claims.ID, s.now().UTC()).First(&u).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Principal{}, apierror.Unauthorized
	}
	if err != nil {
		return Principal{}, err
	}
	return Principal{u, claims.ID}, nil
}
func (s *Service) Logout(ctx context.Context, p Principal) error {
	return s.db.WithContext(ctx).Where("id = ? AND user_id = ?", p.SessionID, p.User.ID).Delete(&Session{}).Error
}
