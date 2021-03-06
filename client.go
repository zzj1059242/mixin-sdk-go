package mixin

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-resty/resty/v2"
)

type Client struct {
	Signer
	Verifier

	ClientID string
}

func NewFromKeystore(keystore *Keystore) (*Client, error) {
	var auth *KeystoreAuth
	var err error
	if strings.Contains(keystore.PrivateKey, "RSA PRIVATE KEY") {
		auth, err = AuthFromKeystore(keystore)
		if err != nil {
			return nil, fmt.Errorf("RSA keystore: %w", err)
		}
	} else if _, err := ed25519Encoding.DecodeString(keystore.PrivateKey); err == nil {
		auth, err = AuthEd25519FromKeystore(keystore)
		if err != nil {
			return nil, fmt.Errorf("ed25519 keystore: %w", err)
		}
	} else {
		return nil, fmt.Errorf("unexpected private key format")
	}

	c := &Client{
		Signer:   auth,
		Verifier: NopVerifier(),
		ClientID: keystore.ClientID,
	}

	return c, nil
}

func NewFromAccessToken(accessToken string) *Client {
	c := &Client{
		Signer:   accessTokenAuth(accessToken),
		Verifier: NopVerifier(),
	}

	return c
}

func NewFromOauthKeystore(keystore *OauthKeystore) (*Client, error) {
	auth, err := AuthFromOauthKeystore(keystore)
	if err != nil {
		return nil, err
	}

	c := &Client{
		Signer:   auth,
		Verifier: auth,
		ClientID: keystore.ClientID,
	}

	return c, nil
}

func (c *Client) Request(ctx context.Context) *resty.Request {
	ctx = WithVerifier(ctx, c.Verifier)
	ctx = WithSigner(ctx, c.Signer)
	return Request(ctx)
}

func (c *Client) Get(ctx context.Context, uri string, params map[string]string, resp interface{}) error {
	r, err := c.Request(ctx).SetQueryParams(params).Get(uri)
	if err != nil {
		return err
	}

	return UnmarshalResponse(r, resp)
}

func (c *Client) Post(ctx context.Context, uri string, body interface{}, resp interface{}) error {
	r, err := c.Request(ctx).SetBody(body).Post(uri)
	if err != nil {
		return err
	}

	return UnmarshalResponse(r, resp)
}
