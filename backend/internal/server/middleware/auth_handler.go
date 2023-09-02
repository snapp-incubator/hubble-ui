package middleware

import (
	"context"
	"errors"
	"fmt"
	"k8s.io/client-go/rest"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cilium/hubble-ui/backend/internal/config"
	"github.com/cilium/hubble-ui/backend/pkg/logger"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/golang-jwt/jwt/v4"
	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2"

	projectv1 "github.com/openshift/client-go/project/clientset/versioned/typed/project/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var (
	log       = logger.New("dex-login")
	serverErr = errors.New("server_error")
)

type (
	DexAuthHandler struct {
		cfg config.DexConfig
		K8SClusterConfig *rest.Config
	}

	user struct {
		ID            string   `bson:"_id,omitempty" json:"_id"`
		UserName      string   `bson:"username,omitempty" json:"username"`
		Password      string   `bson:"password,omitempty" json:"password,omitempty"`
		Email         string   `bson:"email,omitempty" json:"email,omitempty"`
		Name          string   `bson:"name,omitempty" json:"name,omitempty"`
		Groups        []string `bson:"groups,omitempty" json:"groups,omitempty"`
		Namespaces    string   `bson:"namespaces,omitempty" json:"namespaces,omitempty"`
		CreatedAt     *string  `bson:"created_at,omitempty" json:"created_at,omitempty"`
		UpdatedAt     *string  `bson:"updated_at,omitempty" json:"updated_at,omitempty"`
		DeactivatedAt *string  `bson:"deactivated_at,omitempty" json:"deactivated_at,omitempty"`
		Projects      string   `bson:"namespaces,omitempty" json:"namespaces,omitempty"`
	}
)

func NewDex(cfg config.DexConfig) *DexAuthHandler {
	return &DexAuthHandler{cfg: cfg}
}

func (h DexAuthHandler) AuthMiddleware(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(resp http.ResponseWriter, req *http.Request) {
		jwtToken, exp := h.dexCallBack(req)
		if jwtToken != "" {
			log.Infoln(jwtToken)
			http.SetCookie(resp, &http.Cookie{
				Name:       "token",
				Value:      jwtToken,
				Expires:    *exp,
				Path:       "/",
				RawExpires: exp.String(),
				Secure:     true,
			})
			http.Redirect(resp, req, h.cfg.HubbleURL, http.StatusFound)
			return
		}

		token, err := req.Cookie("token")
		if err == nil && token.Value != "" {
			next.ServeHTTP(resp, req)
			return
		}

		dexToken, err := h.generateOAuthJWT()
		if err != nil {
			log.Error(err)

			http.Error(resp, serverErr.Error(), http.StatusInternalServerError)
			return
		}

		cfg, _, err := h.oAuthDexConfig()
		if err != nil {
			log.Error(err)
			http.Error(resp, serverErr.Error(), http.StatusInternalServerError)
			return
		}

		url := cfg.AuthCodeURL(dexToken)
		http.Redirect(resp, req, url, http.StatusFound)
	})
}

func (h DexAuthHandler) generateOAuthJWT() (string, error) {
	token := jwt.New(jwt.SigningMethodHS512)
	claims := token.Claims.(jwt.MapClaims)

	exp := time.Now().Add(h.cfg.JWTExpiration)
	claims["exp"] = exp.Unix()

	tokenString, err := token.SignedString([]byte(h.cfg.Secret))
	if err != nil {
		logrus.Info(err)
		return "", err
	}

	return tokenString, nil
}

func (h DexAuthHandler) oAuthDexConfig() (*oauth2.Config, *oidc.IDTokenVerifier, error) {
	ctx := oidc.ClientContext(context.Background(), &http.Client{})
	provider, err := oidc.NewProvider(ctx, h.cfg.Addr)
	if err != nil {
		log.Errorf("OAuth Error: Something went wrong with OIDC provider %s", err)
		return nil, nil, err
	}

	return &oauth2.Config{
		RedirectURL:  h.cfg.HubbleURL + "/api/",
		ClientID:     h.cfg.ClientID,
		ClientSecret: h.cfg.Secret,
		Scopes:       []string{"openid", "profile", "email", "groups"},
		Endpoint:     provider.Endpoint(),
	}, provider.Verifier(&oidc.Config{ClientID: h.cfg.ClientID}), nil
}

func (h DexAuthHandler) dexCallBack(req *http.Request) (string, *time.Time) {
	incomingState, ok := req.URL.Query()["state"]
	if !ok || len(incomingState) == 0 {
		return "", nil
	}

	validated, err := h.validateOAuthJWT(incomingState[0])
	if !validated {
		return "", nil
	}

	cfg, verifier, err := h.oAuthDexConfig()
	if err != nil {
		log.Error(err)
		return "", nil
	}

	code, ok := req.URL.Query()["code"]
	if !ok || len(code) == 0 {
		return "", nil
	}

	token, err := cfg.Exchange(context.Background(), code[0])
	if err != nil {
		log.Error(err)
		return "", nil
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok {
		log.Error("OAuth Error: no raw id_token found")
		return "", nil
	}

	idToken, err := verifier.Verify(req.Context(), rawIDToken)
	if err != nil {
		log.Error("OAuth Error: no id_token found")
		return "", nil
	}

	var claims struct {
		ID       string `json:"openid"`
		Name     string
		Email    string   `json:"email"`
		Verified bool     `json:"email_verified"`
		Groups   []string `json:"groups"`
	}
	if err := idToken.Claims(&claims); err != nil {
		log.Error("OAuth Error: claims not found")
		return "", nil
	}

	// ocCmd := []string{"oc", "--as", claims.Name, "projects"}
	// cmd := exec.Command(ocCmd[0], ocCmd[1:]...)
	// var outbuf, errbuf strings.Builder
	// cmd.Stdout = &outbuf
	// cmd.Stderr = &errbuf
	// err = cmd.Run()
	// if err != nil || errbuf.String() != "" {
	// 	log.Infoln(err)
	// 	return "", nil
	// }

	projects, err := h.getProjects(claims.Name)
	if err != nil {
		log.Errorf("Get Projects Error:%s", err)
		return "", nil
	}


	createdAt := strconv.FormatInt(time.Now().Unix(), 10)

	var userData = &user{
		ID:        	claims.ID,
		Name:       claims.Name,
		Email:      claims.Email,
		UserName:   claims.Email,
		Groups:     claims.Groups,
		Projects:  strings.Join(projects, ","),
		CreatedAt:  &createdAt,
	}

	jwtToken, exp, err := h.GetSignedJWT(userData)
	if err != nil {
		log.Error(err)
		return "", nil
	}

	return jwtToken, exp
}

func (h DexAuthHandler) validateOAuthJWT(tokenString string) (bool, error) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, isValid := token.Method.(*jwt.SigningMethodHMAC); !isValid {
			return nil, fmt.Errorf("invalid token %s", token.Header["alg"])
		}
		return []byte(h.cfg.Secret), nil
	})

	if err != nil {
		return false, err
	}

	if _, ok := token.Claims.(jwt.Claims); !ok && !token.Valid {
		return false, err
	}

	return true, nil
}

func (h DexAuthHandler) GetSignedJWT(user *user) (string, *time.Time, error) {
	token := jwt.New(jwt.SigningMethodHS512)
	claims := token.Claims.(jwt.MapClaims)
	claims["username"] = user.UserName

	iat := time.Now()
	exp := iat.Add(h.cfg.JWTExpiration)

	claims["iat"] = iat.Unix()
	claims["exp"] = exp.Unix()

	tokenString, err := token.SignedString([]byte(h.cfg.Secret))
	if err != nil {
		logrus.Info(err)
		return "", nil, err
	}

	return tokenString, &exp, nil
}
func (h DexAuthHandler) getProjects(username string) ([]string, error) {
	h.K8SClusterConfig.Impersonate.UserName = username
	projectClientset, err := projectv1.NewForConfig(h.K8SClusterConfig)
	if err != nil {
		logrus.Error(err)
		return nil, err
	}

	res, err := projectClientset.Projects().List(context.Background(), metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	//projects := make(map[string]struct{})
	projects := []string{}
	for _, item := range res.Items {
		projects = append(projects, item.ObjectMeta.Name)
	}

	return projects, err
}

func verifyNS(token, ns string) bool {
	jwtToken, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{})
	if err != nil {
		log.Error(err)
		return false
	}

	if claims, ok := jwtToken.Claims.(jwt.MapClaims); ok {
		projects := strings.Split(claims["projects"].(string), ",")
		if contains(projects, ns) {
			return true
		}
	}

	return false
}

func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}

	return false
}

