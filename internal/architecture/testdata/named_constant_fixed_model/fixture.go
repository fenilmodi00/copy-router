package namedconstantfixedmodel

import (
	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
)

const fixedModel = "moonshotai/kimi-k3"

func decision() router.Decision {
	return router.Decision{Provider: providers.ProviderAIAND, Model: fixedModel}
}
