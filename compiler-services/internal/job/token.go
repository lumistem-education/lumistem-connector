package job

import (
    "errors"
    "time"

    jwt "github.com/golang-jwt/jwt/v5"
)

type TokenScope string

const (
    ScopeWS       TokenScope = "workspace"
    ScopeDownload TokenScope = "download"
)

func generateToken(secret, jobID string, scope TokenScope, ttl time.Duration) (string, error) {
    claims := jwt.MapClaims{
        "job_id": jobID,
        "scope":  string(scope),
        "exp":    time.Now().Add(ttl).Unix(),
    }
    token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
    return token.SignedString([]byte(secret))
}

func verifyToken(secret, tokenStr string, wantScope TokenScope) (string, error) {
    token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
        if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, errors.New("unexpected signing method")
        }
        return []byte(secret), nil
    })
    if err != nil {
        return "", err
    }
    if !token.Valid {
        return "", errors.New("invalid token")
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok {
        return "", errors.New("invalid claims")
    }

    scope, ok := claims["scope"].(string)
    if !ok || TokenScope(scope) != wantScope {
        return "", errors.New("invalid token scope")
    }

    jid, ok := claims["job_id"].(string)
    if !ok || jid == "" {
        return "", errors.New("job_id claim missing")
    }

    return jid, nil
}

func GenerateSSEToken(secret, jobID string, ttl time.Duration) (string, error) {
    return generateToken(secret, jobID, ScopeWS, ttl)
}

func VerifySSEToken(secret, tokenStr string) (string, error) {
    return verifyToken(secret, tokenStr, ScopeWS)
}

func GenerateDownloadToken(secret, jobID string, ttl time.Duration) (string, error) {
    return generateToken(secret, jobID, ScopeDownload, ttl)
}

func VerifyDownloadToken(secret, tokenStr string) (string, error) {
    return verifyToken(secret, tokenStr, ScopeDownload)
}

func GenerateWorkspaceToken(secret, jobID string, ttl time.Duration) (string, error) {
    return generateToken(secret, jobID, ScopeWS, ttl)
}

func VerifyWorkspaceToken(secret, tokenStr string) (string, error) {
    return verifyToken(secret, tokenStr, ScopeWS)
}
