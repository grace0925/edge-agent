/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package login

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/cookiejar"
	"strings"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
	"github.com/trustbloc/edge-agent/test/bdd/pkg/bddcontext"
)

// HTTP server.
const (
	host       = "https://localhost:8091"
	loginPath  = host + "/oidc/login"
	walletPath = host + "/wallet/"
)

// Mock Login Consent App.
const (
	mockLoginEndpoint = "https://localhost:8099/mock/login"
	mockAuthNEndpoint = "https://localhost:8099/mock/authn"
	mockAuthZEndpoint = "https://localhost:8099/mock/authz"
)

// AuthConfigRequest can be sent to mockAuthNEndpoint to configure the user authentication behaviour.
type AuthConfigRequest struct {
	Sub  string `json:"sub"`
	Fail bool   `json:"fail,omitempty"`
}

// ConsentConfigRequest can be sent to mockAuthZEndpoint to configure the user authorization behaviour.
type ConsentConfigRequest struct {
	UserClaims *UserClaims `json:"user_claims,omitempty"`
	Fail       bool        `json:"fail,omitempty"`
}

// UserClaims can be used by BDD tests to configure the claims in the id_token.
type UserClaims struct {
	Sub        string `json:"sub"`
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Email      string `json:"email"`
}

type httpResponse struct {
	statusCode int
	body       string
	url        string
}

// NewSteps returns login BDD test steps.
func NewSteps(ctx *bddcontext.BDDContext) *Steps {
	return &Steps{ctx: ctx}
}

// Steps are the login BDD test steps.
type Steps struct {
	browser             *http.Client
	ctx                 *bddcontext.BDDContext
	loginRedirectResult *httpResponse
	expectedUserClaims  *UserClaims
	authNResult         *httpResponse
	authZResult         *httpResponse
}

// Register the login BDD test steps in the godog suite.
func (s *Steps) Register(gs *godog.Suite) {
	gs.Step("the user clicks on the Login button", s.userClicksLoginButton)
	gs.Step("the user is redirected to the OIDC provider", s.userRedirectedToOIDCProvider)
	gs.Step("the user is authenticated", s.userIsAuthenticated)
	gs.Step("the user consents to sharing their identity data", s.userAuthorizesAccessToTheirData)
	gs.Step("the user is redirected to the wallet's dashboard", s.userRedirectedToWallet)
}

func (s *Steps) userClicksLoginButton() error {
	cookieJar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("failed to initialize the user's cookie jar : %w", err)
	}

	s.browser = &http.Client{
		Transport: &http.Transport{TLSClientConfig: s.ctx.TLSConfig},
		Jar:       cookieJar,
	}

	resp, err := s.browser.Get(loginPath) // nolint:noctx // no need to set context
	if err != nil {
		return fmt.Errorf("failed to invoke http server login endpoint %s: %w", loginPath, err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	err = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to close HTTP response body: %w", err)
	}

	s.loginRedirectResult = &httpResponse{
		statusCode: resp.StatusCode,
		body:       string(body),
		url:        resp.Request.URL.String(),
	}

	return nil
}

func (s *Steps) userRedirectedToOIDCProvider() error {
	if s.loginRedirectResult.statusCode != http.StatusOK {
		return fmt.Errorf(
			"expected StatusCode=%d on login redirection but got %d", http.StatusOK, s.loginRedirectResult.statusCode)
	}

	if s.loginRedirectResult.body != "mock UI" {
		return fmt.Errorf("unexpected UI rendered: %s", s.loginRedirectResult.body)
	}

	if !strings.HasPrefix(s.loginRedirectResult.url, mockLoginEndpoint) {
		return fmt.Errorf("expected login URL %s but got %s", mockLoginEndpoint, s.loginRedirectResult.url)
	}

	return nil
}

func (s *Steps) userIsAuthenticated() error {
	s.expectedUserClaims = &UserClaims{
		Sub:        uuid.New().String(),
		Name:       "John Doe",
		GivenName:  "John",
		FamilyName: "Doe",
		Email:      "john.doe@test.com",
	}

	request, err := json.Marshal(&AuthConfigRequest{
		Sub: s.expectedUserClaims.Sub,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal auth config request: %w", err)
	}

	// nolint:noctx // no need to set the context
	resp, err := s.browser.Post(mockAuthNEndpoint, "application/json", bytes.NewReader(request))
	if err != nil {
		return fmt.Errorf("failed to post request to mock auth endpoint: %w", err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	err = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to close response body: %w", err)
	}

	s.authNResult = &httpResponse{
		statusCode: resp.StatusCode,
		body:       string(body),
		url:        resp.Request.URL.String(),
	}

	return nil
}

func (s *Steps) userAuthorizesAccessToTheirData() error {
	request, err := json.Marshal(&ConsentConfigRequest{
		UserClaims: s.expectedUserClaims,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal consent config request: %w", err)
	}

	// nolint:noctx // no need to set the context
	resp, err := s.browser.Post(mockAuthZEndpoint, "application/json", bytes.NewReader(request))
	if err != nil {
		return fmt.Errorf("failed to invoke mock authZ endpoint %s: %w", mockAuthZEndpoint, err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	err = resp.Body.Close()
	if err != nil {
		return fmt.Errorf("failed to close http response body: %w", err)
	}

	s.authZResult = &httpResponse{
		statusCode: resp.StatusCode,
		body:       string(body),
		url:        resp.Request.URL.String(),
	}

	return nil
}

func (s *Steps) userRedirectedToWallet() error {
	if s.authZResult.statusCode != http.StatusOK {
		fmt.Printf("url: %s\n", s.authZResult.url)
		fmt.Printf("body: %s\n", s.authZResult.body)

		return fmt.Errorf("expected status code %d but got %d", http.StatusFound, s.authZResult.statusCode)
	}

	if !strings.HasPrefix(s.authZResult.url, walletPath) {
		return fmt.Errorf("expected path %s but got %s", walletPath, s.authZResult.url)
	}

	return nil
}
