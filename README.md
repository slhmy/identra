# Identra

Identra is an out-of-the-box authentication and user management service designed to simplify the user
authentication process for applications.

## Features

- 🔐 **Multiple Authentication Methods**: OAuth (GitHub), Email Code, and Password-based authentication
- 🔑 **JWT Token Management**: Industry-standard JWT tokens with JWKS support for secure token validation
- 🔄 **Token Refresh**: Long-lived refresh tokens for seamless user sessions
- 🔗 **Account Linking**: Link multiple authentication methods to a single user account
- 🗄️ **Flexible Storage**: Support for PostgreSQL, MySQL, and SQLite databases
- 📧 **Email Integration**: Configurable SMTP for email-based authentication
- 🚀 **Production Ready**: Docker support, key rotation, and comprehensive security features

## Quick Start

### Running Identra

The easiest way to run Identra is using Docker:

```bash
# Run the gRPC service
docker run -p 50051:50051 -v $(pwd)/config.toml:/app/config.toml ghcr.io/poly-workshop/identra:latest

# Run the HTTP Gateway (in another terminal)
docker run -p 8080:8080 -v $(pwd)/config.toml:/app/config.toml \
  -e SERVICE=identra-gateway ghcr.io/poly-workshop/identra:latest
```

Or build and run from source:

```bash
# Install dependencies
go mod download

# Run gRPC service
go run ./cmd/identra-grpc

# Run HTTP Gateway (in another terminal)
go run ./cmd/identra-gateway
```

### Configuration

Identra ships with local-friendly defaults in code. Use the root `config.toml` only for values that need to differ from those defaults, such as provider credentials, SMTP settings, or a production database:

```toml
[auth.github]
client_id = "your-github-client-id"
client_secret = "your-github-client-secret"

[smtp_mailer]
host = "smtp.example.com"
port = 587
username = "your-email@example.com"
password = "your-password"
from_email = "noreply@example.com"
```

Default local values include `grpc_port = 50051`, `http_port = 8080`, `grpc_endpoint = "localhost:50051"` for the gateway, Redis at `localhost:6379`, SQLite at `data/users.db`, `log.format = "tint"`, and 15-minute access / 7-day refresh tokens.

For browser clients on other origins, set explicit CORS origins:

```toml
[cors]
allowed_origins = ["https://app.example.com"]
allow_credentials = true
```

## Integrating Identra with Your Service

Identra provides both HTTP REST API and gRPC interfaces for integration.

### Integration Options

#### Option 1: HTTP REST API (Recommended for most web applications)

The HTTP Gateway provides RESTful endpoints that are easy to integrate with any language or framework.

**Base URL**: `http://localhost:8080` (or your deployed instance)

#### Option 2: gRPC (For high-performance microservices)

Use the gRPC service directly for better performance in microservice architectures.

**Endpoint**: `localhost:50051`

### Authentication Flow Examples

#### 1. OAuth Authentication (GitHub)

```javascript
// Step 1: Get OAuth authorization URL
const response = await fetch('http://localhost:8080/oauth/url?provider=github');
const { url, state } = await response.json();

// Step 2: Redirect user to GitHub
window.location.href = url;

// Step 3: Handle callback (after user authorizes)
// GitHub redirects to your callback URL with code and state
const loginResponse = await fetch('http://localhost:8080/oauth/login', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ code, state })
});

const { token } = await loginResponse.json();
// token contains: { access_token, refresh_token, token_type }
```

#### 2. Email Code Authentication

```python
import requests

# Step 1: Send login code to user's email
response = requests.post('http://localhost:8080/email/code', json={
    'email': 'user@example.com',
    'use_html': True
})

# Step 2: User receives code via email, then login
login_response = requests.post('http://localhost:8080/email/login', json={
    'email': 'user@example.com',
    'code': '123456'  # Code from email
})

tokens = login_response.json()['token']
access_token = tokens['access_token']['token']
```

#### 3. Password Authentication

```go
package main

import (
    "bytes"
    "encoding/json"
    "net/http"
)

func login(email, password string) (string, error) {
    body, _ := json.Marshal(map[string]string{
        "email": email,
        "password": password,
    })
    
    resp, err := http.Post(
        "http://localhost:8080/password/login",
        "application/json",
        bytes.NewBuffer(body),
    )
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    var result struct {
        Token struct {
            AccessToken struct {
                Token string `json:"token"`
            } `json:"access_token"`
        } `json:"token"`
    }
    
    json.NewDecoder(resp.Body).Decode(&result)
    return result.Token.AccessToken.Token, nil
}
```

### Token Validation

Identra issues JWT tokens that your services can validate independently using the JWKS endpoint.

#### Validating Tokens in Your Service

**Step 1: Fetch the JWKS (JSON Web Key Set)**

```bash
curl http://localhost:8080/.well-known/jwks.json
```

Response:

```json
{
  "keys": [
    {
      "kty": "RSA",
      "alg": "RS256",
      "use": "sig",
      "kid": "key-id-123",
      "n": "...",
      "e": "AQAB"
    }
  ]
}
```

**Step 2: Validate JWT tokens using the public key**

Example in Node.js:

```javascript
const jwt = require('jsonwebtoken');
const jwksClient = require('jwks-rsa');

const client = jwksClient({
  jwksUri: 'http://localhost:8080/.well-known/jwks.json',
  cache: true,
  cacheMaxAge: 3600000 // 1 hour
});

function getKey(header, callback) {
  client.getSigningKey(header.kid, (err, key) => {
    const signingKey = key.publicKey || key.rsaPublicKey;
    callback(null, signingKey);
  });
}

function verifyToken(token) {
  return new Promise((resolve, reject) => {
    jwt.verify(token, getKey, {
      algorithms: ['RS256'],
      issuer: 'identra'
    }, (err, decoded) => {
      if (err) reject(err);
      else resolve(decoded);
    });
  });
}

// Use in middleware
app.use(async (req, res, next) => {
  const authHeader = req.headers.authorization;
  if (!authHeader) return res.status(401).send('No token provided');
  
  const token = authHeader.split(' ')[1]; // Bearer <token>
  
  try {
    const decoded = await verifyToken(token);
    req.user = { id: decoded.user_id };
    next();
  } catch (err) {
    res.status(401).send('Invalid token');
  }
});
```

Example in Python:

```python
from jose import jwt, jwk
import requests

# Cache JWKS
jwks_url = 'http://localhost:8080/.well-known/jwks.json'
jwks = requests.get(jwks_url).json()

def verify_token(token):
    # Decode header to get kid
    unverified_header = jwt.get_unverified_header(token)
    
    # Find the key
    rsa_key = {}
    for key in jwks['keys']:
        if key['kid'] == unverified_header['kid']:
            rsa_key = key
            break
    
    if not rsa_key:
        raise Exception('Public key not found')
    
    # Verify token
    payload = jwt.decode(
        token,
        rsa_key,
        algorithms=['RS256'],
        issuer='identra'
    )
    return payload
```

Example in Go:

```go
package main

import (
    "github.com/golang-jwt/jwt/v5"
    "github.com/lestrrat-go/jwx/jwk"
)

func validateToken(tokenString string) (*jwt.Token, error) {
    // Fetch JWKS
    set, err := jwk.Fetch(context.Background(), 
        "http://localhost:8080/.well-known/jwks.json")
    if err != nil {
        return nil, err
    }
    
    // Parse and validate token
    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        // Get key ID from token
        kid, ok := token.Header["kid"].(string)
        if !ok {
            return nil, fmt.Errorf("kid not found")
        }
        
        // Find the key
        key, ok := set.LookupKeyID(kid)
        if !ok {
            return nil, fmt.Errorf("key not found")
        }
        
        // Convert to public key
        var pubkey interface{}
        if err := key.Raw(&pubkey); err != nil {
            return nil, err
        }
        return pubkey, nil
    })
    
    return token, err
}
```

### Token Refresh

Access tokens are short-lived (typically 15 minutes). Use refresh tokens to obtain new access tokens:

```javascript
async function refreshAccessToken(refreshToken) {
  const response = await fetch('http://localhost:8080/token/refresh', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ refresh_token: refreshToken })
  });
  
  const { token } = await response.json();
  return token.access_token.token;
}
```

Refresh tokens are rotated: once a refresh token is used successfully, Identra revokes that refresh token until its original expiry.

To revoke a refresh token during logout:

```bash
curl -X POST http://localhost:8080/token/revoke \
  -H "Content-Type: application/json" \
  -d '{"refresh_token": "your-refresh-token"}'
```

### Getting User Information

Retrieve information about the currently authenticated user:

```bash
curl -X POST http://localhost:8080/me/login-info \
  -H "Authorization: Bearer your-access-token" \
  -H "Content-Type: application/json" \
  -d '{}'
```

Response:

```json
{
  "user_id": "uuid-here",
  "email": "user@example.com",
  "password_enabled": true,
  "github_id": "123456",
  "oauth_connections": [
    {
      "provider": "github",
      "provider_user_id": "123456"
    }
  ]
}
```

### Account Linking

Users can link multiple authentication methods to their account:

```javascript
// User is already logged in with one method
const accessToken = 'current-access-token';

// Get OAuth URL for linking
const { url, state } = await fetch(
  'http://localhost:8080/oauth/url?provider=github'
).then(r => r.json());

// After OAuth callback, bind the account
await fetch('http://localhost:8080/oauth/bind', {
  method: 'POST',
  headers: {
    'Authorization': `Bearer ${accessToken}`,
    'Content-Type': 'application/json'
  },
  body: JSON.stringify({
    code: oauthCode,
    state: state
  })
});
```

## API Reference

For a complete API reference, see the OpenAPI specification at `gen/openapi/identra.swagger.json` or the Protocol Buffer definitions in `proto/identra/v1/`.

### Main Endpoints

- `GET /healthz` - Gateway process liveness
- `GET /readyz` - Gateway readiness, including upstream gRPC health
- `GET /.well-known/jwks.json` - Get JSON Web Key Set for token validation
- `GET /oauth/url` - Get OAuth authorization URL
- `POST /oauth/login` - Login via OAuth
- `POST /oauth/bind` - Bind OAuth account to existing user
- `POST /email/code` - Send login code via email
- `POST /email/login` - Login with email code
- `POST /password/register` - Register with email and password
- `POST /password/login` - Login with email and password
- `POST /token/refresh` - Refresh access token
- `POST /token/revoke` - Revoke a refresh token
- `POST /me/login-info` - Get current user's login information

## Advanced Topics

### Key Rotation

For production deployments, Identra supports JWT signing key rotation. See [docs/KEY_ROTATION.md](./docs/KEY_ROTATION.md) for detailed procedures.

### Database Setup

Identra supports multiple databases. Example PostgreSQL configuration:

```toml
[persistence.gorm]
driver = "postgres"
host = "localhost"
port = 5432
dbname = "identra"
username = "identra_user"
password = "secure_password"
sslmode = "require"
```

### Production Deployment

1. **Use environment variables** for sensitive configuration
2. **Enable HTTPS** for all endpoints
3. **Configure CORS** appropriately for your frontend domains
4. **Set up monitoring** for token validation failures
5. **Implement rate limiting** on authentication endpoints
6. **Regular key rotation** following the documented procedures
7. **Use Redis** for session storage in distributed deployments

## Contributing

Please refer to [CONTRIBUTING.md](./CONTRIBUTING.md) for contribution guidelines,
or check the documentation in the [docs](./docs) directory for more details on project design.
