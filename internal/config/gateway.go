package config

import "github.com/slhmy/identra/internal/bootstrap"

type GatewayConfig struct {
	HTTPPort     uint
	GRPCEndpoint string
	CORS         CORSConfig
}

type CORSConfig struct {
	AllowedOrigins   []string
	AllowCredentials bool
}

func LoadGateway() GatewayConfig {
	return GatewayConfig{
		HTTPPort:     bootstrap.Config().GetUint(HTTPPortKey),
		GRPCEndpoint: bootstrap.Config().GetString(GRPCEndpointKey),
		CORS: CORSConfig{
			AllowedOrigins:   bootstrap.Config().GetStringSlice(CORSAllowedOriginsKey),
			AllowCredentials: bootstrap.Config().GetBool(CORSAllowCredentialsKey),
		},
	}
}
