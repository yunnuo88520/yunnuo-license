package licensing

import (
	"context"
	yncrypto "github.com/yunnuo88520/yunnuo-license/backend/internal/crypto"
	"strconv"
	"strings"
	"time"
)

func (s *Service) PublicKeys(ctx context.Context, appID string) (JWKS, error) {
	app, e := s.registry.GetApplication(ctx, appID)
	if e != nil {
		return JWKS{}, notFound()
	}
	scoped, e := BindScope(ctx, app.ApplicationID, Actor{Kind: "public_key_read", ID: "jwks"}, false)
	if e != nil {
		return JWKS{}, e
	}
	repo, e := s.registry.Scoped(scoped)
	if e != nil {
		return JWKS{}, e
	}
	result := JWKS{Keys: []JWK{}}
	e = repo.View(scoped, func(view View) error {
		keys, e := view.Keys().Find(scoped, Filter{Limit: 1000})
		if e != nil {
			return e
		}
		for _, key := range keys {
			if key.Status != "active" && (key.Status != "retired" || key.AcceptUntil == nil || !s.now().Before(*key.AcceptUntil)) {
				continue
			}
			if !strings.HasPrefix(key.KID, appID+".v") {
				return ErrScopeMismatch
			}
			public, e := PublicJWK(key.PublicPEM, key.KID)
			if e != nil {
				return e
			}
			result.Keys = append(result.Keys, public)
		}
		return nil
	})
	return result, e
}
func (s *Service) RotateSigningKey(ctx context.Context, id, reason string, retireImmediately bool) (map[string]any, error) {
	scoped, app, repo, e := s.applicationContext(ctx, id, true)
	if e != nil {
		return nil, e
	}
	if strings.TrimSpace(reason) == "" || len(reason) > 500 {
		return nil, invalid("请填写密钥轮换原因")
	}
	public, private, e := yncrypto.GenerateRSAKeyPair()
	if e != nil {
		return nil, e
	}
	cipher, e := yncrypto.EncryptString(s.cryptoKey(id, "signing"), private)
	if e != nil {
		return nil, e
	}
	var created SigningKey
	var oldKID string
	e = repo.Update(scoped, func(tx Transaction) error {
		current, e := tx.Application(scoped)
		if e != nil {
			return e
		}
		app = current
		old, e := tx.Keys().Get(scoped, app.CurrentKID)
		if e != nil {
			return e
		}
		oldKID = old.KID
		until := s.now().Add(time.Duration(app.VerifyPolicy.KeyOverlapSeconds) * time.Second)
		if old.MaxIssuedUntil != nil && old.MaxIssuedUntil.After(until) {
			until = *old.MaxIssuedUntil
		}
		if retireImmediately {
			until = s.now()
		}
		old.Status = "retired"
		old.AcceptUntil = &until
		old.PrivateCipher = ""
		old.UpdatedAt = s.now()
		if e = tx.Keys().Save(scoped, old); e != nil {
			return e
		}
		created = SigningKey{Record: s.record(id, "key"), KID: id + ".v" + strconv.Itoa(old.Version+1), Version: old.Version + 1, Algorithm: "RS256", PublicPEM: public, PrivateCipher: cipher, Status: "active"}
		created.ID = created.KID
		if e = tx.Keys().Insert(scoped, created); e != nil {
			return e
		}
		app.CurrentKID = created.KID
		app.UpdatedAt = s.now()
		if e = tx.SaveApplication(scoped, app); e != nil {
			return e
		}
		return s.append(scoped, tx, "signing_key_rotated", created.KID, map[string]any{"reason": reason, "previous_kid": old.KID, "overlap_until": until, "retire_immediately": retireImmediately})
	})
	return map[string]any{"app_id": id, "kid": created.KID, "previous_kid": oldKID, "public_key_pem": public}, e
}
func (s *Service) signClaims(ctx context.Context, tx Transaction, app Application, claims JWTClaims) (string, error) {
	key, e := tx.Keys().Get(ctx, app.CurrentKID)
	if e != nil {
		return "", e
	}
	if key.Status != "active" || key.PrivateCipher == "" {
		return "", forbidden("SIGNING_KEY_UNAVAILABLE", "当前签名密钥不可用")
	}
	private, e := yncrypto.DecryptString(s.cryptoKey(app.ApplicationID, "signing"), key.PrivateCipher)
	if e != nil {
		return "", e
	}
	token, e := SignJWT(private, key.KID, claims)
	if e != nil {
		return "", e
	}
	until := time.Unix(claims.ExpiresAt, 0).UTC()
	if key.MaxIssuedUntil == nil || key.MaxIssuedUntil.Before(until) {
		key.MaxIssuedUntil = &until
		key.UpdatedAt = s.now()
		if e = tx.Keys().Save(ctx, key); e != nil {
			return "", e
		}
	}
	return token, nil
}
func (s *Service) verifyApplicationToken(ctx context.Context, keys ReadTable[SigningKey], app Application, token string, expected TokenExpectation, association bool) (JWTClaims, error) {
	header, e := DecodeJWTHeader(token)
	if e != nil {
		return JWTClaims{}, e
	}
	if !strings.HasPrefix(header.KID, app.ApplicationID+".v") {
		return JWTClaims{}, tokenError("TOKEN_KEY_SCOPE_INVALID")
	}
	key, e := keys.Get(ctx, header.KID)
	if e != nil {
		return JWTClaims{}, tokenError("TOKEN_KEY_UNKNOWN")
	}
	if key.Status != "active" && (key.Status != "retired" || key.AcceptUntil == nil || !s.now().Before(*key.AcceptUntil)) {
		return JWTClaims{}, tokenError("TOKEN_KEY_RETIRED")
	}
	if association {
		return verifyJWTForRefresh(token, key.PublicPEM, expected)
	}
	return VerifyJWT(token, key.PublicPEM, expected)
}
